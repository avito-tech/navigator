package resources

import (
	"sort"
	"sync"

	"github.com/avito-tech/navigator/pkg/grpc"
	"github.com/avito-tech/navigator/pkg/k8s"

	api "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	"github.com/envoyproxy/go-control-plane/envoy/api/v2/core"
	"github.com/envoyproxy/go-control-plane/envoy/api/v2/route"
	"github.com/envoyproxy/go-control-plane/pkg/cache"
	"github.com/gogo/protobuf/proto"
	"github.com/gogo/protobuf/types"
)

type RouteConfigurationByCluster struct {
	ClusterID          string
	RouteConfiguration *api.RouteConfiguration
}

// RouteCache manages the contents of the gRPC RDS cache.
type RouteCache struct {
	HealthCheckConfig

	observable

	mu                  sync.Mutex
	routeNamesByService map[k8s.QualifiedName][]RouteConfigurationByCluster
}

type clusterRouteCache struct {
	*RouteCache
	clusterID string
}

// NewRouteCache returns an instance of a RouteCache
func NewRouteCache(opts ...FuncOpt) *RouteCache {
	r := &RouteCache{
		routeNamesByService: map[k8s.QualifiedName][]RouteConfigurationByCluster{},
	}
	r.EnableHealthCheck = false
	SetOpts(r, opts...)
	return r
}

func (c *RouteCache) Get(clusterID string) (resource grpc.Resource, ok bool) {
	clustered := &clusterRouteCache{RouteCache: c, clusterID: clusterID}
	return clustered, true
}

func (c *RouteCache) AddService(service *k8s.Service) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	serviceKey := service.Key()
	c.removeServiceByName(serviceKey)

	for _, p := range service.Ports {
		if p.Protocol != k8s.ProtocolTCP {
			continue
		}

		for _, clusterIP := range service.ClusterIPs {
			name := ListenerName(clusterIP.IP, p.Port)
			clusters := weightedClusters(p.Name, service.BackendsBySourceID)
			routeConf := routeConfiguration(name, clusters)
			c.routeNamesByService[serviceKey] = append(c.routeNamesByService[serviceKey], RouteConfigurationByCluster{
				ClusterID:          clusterIP.ClusterID,
				RouteConfiguration: routeConf,
			})
		}
	}

	return nil
}

func (c *RouteCache) RemoveService(serviceKey k8s.QualifiedName) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.removeServiceByName(serviceKey)
	return nil
}

func (c *RouteCache) removeServiceByName(serviceKey k8s.QualifiedName) {
	c.routeNamesByService[serviceKey] = nil
}

func (c *RouteCache) getNameToRouteMapping() map[string]map[string]*api.RouteConfiguration {
	mapping := make(map[string]map[string]*api.RouteConfiguration)

	for _, v := range c.routeNamesByService {
		for _, r := range v {
			if _, ok := mapping[r.ClusterID]; !ok {
				mapping[r.ClusterID] = make(map[string]*api.RouteConfiguration)
			}
			mapping[r.ClusterID][r.RouteConfiguration.Name] = r.RouteConfiguration
		}
	}

	return mapping
}

// Contents returns a copy of the cache's contents.
func (c *clusterRouteCache) Contents() []proto.Message {
	c.mu.Lock()
	defer c.mu.Unlock()
	var values []proto.Message

	health := healthRouteConfiguration(c.HealthCheckName, c.HealthCheckPath)
	values = append(values, health)
	for _, v := range c.getNameToRouteMapping()[c.clusterID] {
		values = append(values, v)
	}
	sort.Stable(routesByName(values))
	return values

}

func (c *clusterRouteCache) Query(names []string) []proto.Message {
	c.mu.Lock()
	defer c.mu.Unlock()

	mapping := c.getNameToRouteMapping()
	var values []proto.Message
	for _, n := range names {
		v, ok := mapping[c.clusterID][n]
		if ok {
			values = append(values, v)
		}
		if n == c.HealthCheckName {
			health := healthRouteConfiguration(c.HealthCheckName, c.HealthCheckPath)
			values = append(values, health)
		}
	}
	sort.Stable(routesByName(values))
	return values
}

func (*RouteCache) TypeURL() string { return cache.RouteType }

type routesByName []proto.Message

func (l routesByName) Len() int      { return len(l) }
func (l routesByName) Swap(i, j int) { l[i], l[j] = l[j], l[i] }
func (l routesByName) Less(i, j int) bool {
	return l[i].(*api.RouteConfiguration).Name < l[j].(*api.RouteConfiguration).Name
}

func routeConfiguration(name string, clusters *route.WeightedCluster) *api.RouteConfiguration {
	return &api.RouteConfiguration{
		Name: name,
		VirtualHosts: []route.VirtualHost{{
			Name:    name,
			Domains: []string{"*"},
			Routes: []route.Route{{
				Match: route.RouteMatch{
					PathSpecifier: &route.RouteMatch_Prefix{Prefix: "/"},
				},
				Action: &route.Route_Route{
					Route: &route.RouteAction{
						ClusterSpecifier: &route.RouteAction_WeightedClusters{
							WeightedClusters: clusters,
						},
					},
				},
			}},
		}},
	}
}

func weightedClusters(portName string, backendsBySourceID map[k8s.BackendSourceID]k8s.WeightedAddressSet) *route.WeightedCluster {
	clusterWeight := map[string]int{}
	for backend, weightedAdressSet := range backendsBySourceID {
		clusterWeight[backend.ClusterID] += weightedAdressSet.Weight
	}

	var totalWeight uint32
	clusters := []*route.WeightedCluster_ClusterWeight{}
	for backend, weightedAddressSet := range backendsBySourceID {
		// normalize weight (weight/clusterSum * defaultMax)
		weight := int(float32(weightedAddressSet.Weight) /
			float32(clusterWeight[backend.ClusterID]) *
			float32(k8s.EndpointsWeightSumForCluster))
		cluster := &route.WeightedCluster_ClusterWeight{
			Name:   HTTPClusterName(backend, portName),
			Weight: &types.UInt32Value{Value: uint32(weight)},
		}
		totalWeight += uint32(weight)
		clusters = append(clusters, cluster)
	}

	return &route.WeightedCluster{
		TotalWeight: &types.UInt32Value{Value: totalWeight},
		Clusters:    clusters,
	}
}

func healthRouteConfiguration(name, path string) *api.RouteConfiguration {
	return &api.RouteConfiguration{
		Name: name,
		VirtualHosts: []route.VirtualHost{{
			Name:    name,
			Domains: []string{"*"},
			Routes: []route.Route{{
				Match: route.RouteMatch{
					PathSpecifier: &route.RouteMatch_Prefix{Prefix: path},
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
