package resources

import (
	"sync"

	api "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	core "github.com/envoyproxy/go-control-plane/envoy/api/v2/core"
	route "github.com/envoyproxy/go-control-plane/envoy/api/v2/route"
	"github.com/envoyproxy/go-control-plane/pkg/cache"
	"github.com/golang/protobuf/ptypes/duration"
	"github.com/golang/protobuf/ptypes/wrappers"

	"github.com/avito-tech/navigator/pkg/k8s"
)

type RouteCache struct {
	muUpdate sync.Mutex

	HealthCheckConfig

	*BranchedResourceCache
	isGatewayOnly bool
}

func NewRouteCache(opts ...FuncOpt) *RouteCache {
	r := &RouteCache{
		BranchedResourceCache: NewBranchedResourceCache(cache.RouteType),
	}
	r.EnableHealthCheck = false
	SetOpts(r, opts...)

	static := []NamedProtoMessage{}
	if r.EnableHealthCheck {
		static = append(static, r.healthRoute())
	}
	r.SetStaticResources(static)
	return r
}

func (c *RouteCache) UpdateServices(updated, deleted []*k8s.Service) {

	c.muUpdate.Lock()
	defer c.muUpdate.Unlock()

	if c.isGatewayOnly {
		return
	}

	updatedResource := map[string][]NamedProtoMessage{}
	deletedResources := map[string][]NamedProtoMessage{}

	for _, u := range updated {
		c.getServiceRoutes(updatedResource, u)
	}
	for _, d := range deleted {
		c.getServiceRoutes(deletedResources, d)
	}

	c.UpdateBranchedResources(updatedResource, deletedResources)
}

func (c *RouteCache) UpdateIngresses(updated map[k8s.QualifiedName]*k8s.Ingress) {

	c.muUpdate.Lock()
	defer c.muUpdate.Unlock()

	if !c.isGatewayOnly {
		c.isGatewayOnly = true
		c.RenewResourceCache()
	}
	updatedResource := map[string][]NamedProtoMessage{}
	c.getIngressRoutes(updatedResource, updated)
	c.UpdateBranchedResources(updatedResource, nil)
}

func (c *RouteCache) UpdateGateway(gw *k8s.Gateway) {
	// do nothing
}

func (c *RouteCache) healthRoute() *api.RouteConfiguration {
	return &api.RouteConfiguration{
		Name: c.HealthCheckName,
		VirtualHosts: []*route.VirtualHost{{
			Name:    c.HealthCheckName,
			Domains: []string{"*"},
			Routes: []*route.Route{{
				Match: &route.RouteMatch{
					PathSpecifier: &route.RouteMatch_Prefix{Prefix: c.HealthCheckPath},
				},
				Action: &route.Route_DirectResponse{
					DirectResponse: &route.DirectResponseAction{
						Status: 200,
						Body: &core.DataSource{
							Specifier: &core.DataSource_InlineString{
								InlineString: "OK",
							},
						},
					},
				},
			}},
		}},
	}
}

func (c *RouteCache) getIngressRoutes(storage map[string][]NamedProtoMessage, updated map[k8s.QualifiedName]*k8s.Ingress) {

	// map[cluster]map[hostname]map[route]RouteConfig
	routes := map[string]map[string]map[string]*route.Route{}

	for _, ingress := range updated {
		// add cluster for storage if not exists
		if _, ok := storage[ingress.ClusterID]; !ok {
			storage[ingress.ClusterID] = []NamedProtoMessage{}
		}
		// add cluster for routes
		if _, ok := routes[ingress.ClusterID]; !ok {
			routes[ingress.ClusterID] = map[string]map[string]*route.Route{}
		}

		// add rules
		for _, rule := range ingress.Rules {
			if _, ok := routes[ingress.ClusterID][rule.Host]; !ok {
				routes[ingress.ClusterID][rule.Host] = map[string]*route.Route{}
			}

			for _, path := range rule.Paths {
				routeTo := weightedClusters(ingress.Namespace, path.ServiceName, path.PortName)
				var hashPolicy []*route.RouteAction_HashPolicy
				if path.CookieAffinity != nil {
					hashPolicy = routeHashPolicy(*path.CookieAffinity)
				}
				routes[ingress.ClusterID][rule.Host][path.Path] = getRoute(path.Path, path.Rewrite, routeTo, hashPolicy)
			}
		}
	}

	for cluster, hostRules := range routes {
		// create config form routes
		vhosts := []*route.VirtualHost{}
		for domain, rules := range hostRules {
			name := domain
			if len(name) == 0 {
				name = "*"
				// with empty host provided we should match any
				domain = "*"
			}
			vhosts = append(vhosts, getVirtualHost(name, domain, rules))
		}
		r := routeIngressConfiguration(GatewayListenerPrefix, vhosts)
		// add config for cluster
		storage[cluster] = append(storage[cluster], r)
	}
}

func (c *RouteCache) getServiceRoutes(storage map[string][]NamedProtoMessage, service *k8s.Service) {
	for _, p := range service.Ports {
		if p.Protocol != k8s.ProtocolTCP {
			continue
		}

		for _, clusterIP := range service.ClusterIPs {
			name := ListenerNameFromService(service.Namespace, service.Name, p.Port)
			clusters := weightedClusters(service.Namespace, service.Name, p.Name)
			routeConf := routeServiceConfiguration(name, clusters)
			clusterID := clusterIP.ClusterID
			if _, ok := storage[clusterID]; !ok {
				storage[clusterID] = []NamedProtoMessage{}
			}
			storage[clusterID] = append(storage[clusterID], routeConf)
		}
	}
}

func routeServiceConfiguration(name string, clusters *route.WeightedCluster) *api.RouteConfiguration {
	return &api.RouteConfiguration{
		Name: name,
		VirtualHosts: []*route.VirtualHost{{
			Name:    name,
			Domains: []string{"*"},
			Routes: []*route.Route{{
				Match: &route.RouteMatch{
					PathSpecifier: &route.RouteMatch_Prefix{Prefix: "/"},
				},
				Action: &route.Route_Route{
					Route: &route.RouteAction{
						RetryPolicy: retryPolicy(),
						ClusterSpecifier: &route.RouteAction_WeightedClusters{
							WeightedClusters: clusters,
						},
					},
				},
			}},
		}},
	}
}

func getRoute(pathPrefix string, prefixRewrite string, weightedCluster *route.WeightedCluster, hashPolicy []*route.RouteAction_HashPolicy) *route.Route {
	return &route.Route{
		Match: &route.RouteMatch{
			PathSpecifier: &route.RouteMatch_Prefix{Prefix: pathPrefix},
		},
		Action: &route.Route_Route{
			Route: &route.RouteAction{
				IdleTimeout: &duration.Duration{},
				Timeout:     &duration.Duration{},
				HashPolicy:  hashPolicy,
				RetryPolicy: retryPolicy(),
				ClusterSpecifier: &route.RouteAction_WeightedClusters{
					WeightedClusters: weightedCluster,
				},
				PrefixRewrite: prefixRewrite,
			},
		},
	}
}

func weightedClusters(namespace, serviceName, portName string) *route.WeightedCluster {
	return &route.WeightedCluster{
		TotalWeight: &wrappers.UInt32Value{Value: 1},
		Clusters: []*route.WeightedCluster_ClusterWeight{
			{
				Name:   ClusterName(namespace, serviceName, portName),
				Weight: &wrappers.UInt32Value{Value: 1},
			},
		},
	}
}

func getVirtualHost(name, domain string, routes map[string]*route.Route) *route.VirtualHost {
	var r []*route.Route

	for _, ro := range routes {
		r = append(r, ro)
	}

	return &route.VirtualHost{
		Name:    name,
		Domains: []string{domain},
		Routes:  r,
	}
}

func routeIngressConfiguration(name string, vhosts []*route.VirtualHost) *api.RouteConfiguration {
	return &api.RouteConfiguration{
		Name:         name,
		VirtualHosts: vhosts,
	}
}

func routeHashPolicy(cookieName string) []*route.RouteAction_HashPolicy {
	return []*route.RouteAction_HashPolicy{{
		PolicySpecifier: &route.RouteAction_HashPolicy_Cookie_{
			Cookie: &route.RouteAction_HashPolicy_Cookie{
				Name: cookieName,
			},
		},
	}}
}

func retryPolicy() *route.RetryPolicy {
	return &route.RetryPolicy{
		RetryOn: "reset,connect-failure",
	}
}
