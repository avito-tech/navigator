package xds

import (
	"fmt"
	"net/http"
	"regexp"
	"sync"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/avito-tech/navigator/pkg/grpc"
	"github.com/avito-tech/navigator/pkg/k8s"
	"github.com/avito-tech/navigator/pkg/xds/resources"
)

type k8sCache interface {
	// GetSnapshot returns services indexed by namespace and name: services[namespace][name]
	GetSnapshot() (services map[string]map[string]*k8s.Service)
}

type nexusCache interface {
	// GetSnapshot returns nexuses indexed by app Id
	GetSnapshot() (nexusesByAppName map[string]k8s.Nexus, appNamesByService map[k8s.QualifiedName][]string)
}

type gatewayCache interface {
	// GatewayByClass returns all gateways filtered by ingress class
	GatewaysByClass(cluster, ingressClass string) []*k8s.Gateway
}

type ingressCache interface {
	// GetByClass returns ingresses indexed by qualified name filtered by ingress class with clusterID priority
	GetByClass(clusterID string, class string) map[k8s.QualifiedName]*k8s.Ingress
	// GetClusterIDsForIngress returns cluster IDs to understand which gateways should be updated
	GetClusterIDsForIngress(qn k8s.QualifiedName, class string) []string
	GetServicesByClass(clusterID string, class string) []k8s.QualifiedName
}

type Cache struct {
	logger       logrus.FieldLogger
	k8sCache     k8sCache
	nexusCache   nexusCache
	ingressCache ingressCache
	gatewayCache gatewayCache
	resOpts      []resources.FuncOpt

	mu      sync.Mutex
	appXDSs map[string]*AppXDS
}

func NewCache(k8sCache k8sCache, nexusCache nexusCache, ingressCache ingressCache, gatewayCache gatewayCache, logger logrus.FieldLogger, resOpts []resources.FuncOpt) *Cache {
	return &Cache{
		k8sCache:     k8sCache,
		nexusCache:   nexusCache,
		ingressCache: ingressCache,
		gatewayCache: gatewayCache,
		logger:       logger.WithField("context", "envoy.cache"),
		resOpts:      resOpts,
		appXDSs:      make(map[string]*AppXDS),
	}
}

// NotifyGatewayUpdated triggers when gateway updated
func (c *Cache) NotifyGatewayUpdated(updatedGateway *k8s.Gateway, deleted bool) {
	// 1. get all ingresses for ingress class with ClusterID priority
	// 2. get all associated services
	// 3. put gateway entity to GatewayUpdate with all corresponding info
	c.mu.Lock()
	defer c.mu.Unlock()

	if deleted {
		delete(c.appXDSs, updatedGateway.Name)
		return
	}

	if _, ok := c.appXDSs[updatedGateway.Name]; !ok {
		c.appXDSs[updatedGateway.Name] = NewAppXDS(c.logger, c.resOpts)
		ingresses := c.ingressCache.GetByClass(updatedGateway.ClusterID, updatedGateway.IngressClass)
		c.appXDSs[updatedGateway.Name].IngressUpdate(ingresses)
	}

	c.appXDSs[updatedGateway.Name].GatewayUpdate(updatedGateway)
}

// NotifyIngressUpdated triggers when ingress cache updated
func (c *Cache) NotifyIngressUpdated(ingress *k8s.Ingress) {
	gateways := c.gatewayCache.GatewaysByClass(ingress.ClusterID, ingress.Class)
	ingresses := c.ingressCache.GetByClass(ingress.ClusterID, ingress.Class)

	c.mu.Lock()
	defer c.mu.Unlock()

	for _, gw := range gateways {
		if _, ok := c.appXDSs[gw.Name]; !ok {
			c.appXDSs[gw.Name] = NewAppXDS(c.logger, c.resOpts)
		}
		// we don't need pure Update() here cos we already invoked NotifyNexusesUpdated before NotifyIngressUpdated
		c.appXDSs[gw.Name].IngressUpdate(ingresses)
	}
}

// NotifyNexusesUpdated triggers when nexus cache updated and adds/removes AppXDSs
func (c *Cache) NotifyNexusesUpdated(updatedAppNames []string, isGateway bool) {
	nexuses, _ := c.nexusCache.GetSnapshot()
	services := c.k8sCache.GetSnapshot()

	c.mu.Lock()
	defer c.mu.Unlock()

	for _, appName := range updatedAppNames {
		if _, ok := c.appXDSs[appName]; !ok {
			c.appXDSs[appName] = NewAppXDS(c.logger, c.resOpts)
		}

		// if nexus was removed, appNexus is empty
		// it removes all old services from AppXDS
		// and connected envoys will be updated with empty XDS
		c.appXDSs[appName].Update(getAppServices(nexuses[appName], services))
	}

	c.debugPrintCache(fmt.Sprintf("NotifyNexus: %v", updatedAppNames))
}

// NotifyServicesUpdated triggers when k8d cache updated and adds/removes affected services for existing AppXDSs
func (c *Cache) NotifyServicesUpdated(updatedServiceKeys []k8s.QualifiedName) {
	services := c.k8sCache.GetSnapshot()
	nexuses, appNamesByService := c.nexusCache.GetSnapshot()

	c.mu.Lock()
	defer c.mu.Unlock()

	for _, serviceKey := range updatedServiceKeys {
		appNames := append([]string{}, appNamesByService[serviceKey.WholeNamespaceKey()]...)
		appNames = append(appNames, appNamesByService[serviceKey]...)
		for _, appName := range appNames {
			if x, ok := c.appXDSs[appName]; ok {
				// if service disappeared fro k8s services, it will be removed from AppXDS
				// and connected envoys will be updated without this service
				x.Update(getAppServices(nexuses[appName], services))
			} else {
				c.logger.Errorf(
					"Inconsistent envoy cache: appNamesByService[%v] has appName %q, but there are no such appName in c.appXDSs",
					serviceKey, appName,
				)
			}
		}
	}

	c.debugPrintCache(fmt.Sprintf("NotifyK8s: %v", updatedServiceKeys))
}

// GetXDSState handler for envoy resources in xds cache
func (c *Cache) GetAppsEnvoyState(res http.ResponseWriter, _ *http.Request) {
	c.mu.Lock()
	defer c.mu.Unlock()

	var result string
	for appName, app := range c.appXDSs {
		result += fmt.Sprintf("========== App %s ========== \n", appName)
		for t, cres := range app.clusterResources {
			result += fmt.Sprintf("---------- Type %s ---------- \n", t)
			result += cres.String()
		}
		result += "\n"
	}

	_, _ = res.Write([]byte(result))
}

// handler for k8s resources in xds cashe
func (c *Cache) GetAppsK8sState(res http.ResponseWriter, _ *http.Request) {
	c.mu.Lock()
	defer c.mu.Unlock()

	_, _ = res.Write([]byte(c.String()))
}

func (c *Cache) debugPrintCache(msg string) {
	c.logger.WithField("msg", msg).Tracef(`
#################################### %s %s ####################################
%s
############################################################################################################\n\n`,
		time.Now().Format(time.Stamp),
		msg,
		c,
	)
}

func (c *Cache) String() string {
	var result string
	for appName, appXDS := range c.appXDSs {
		result += fmt.Sprintf(
			"=========== App %s ============\n",
			appName,
		)

		for _, service := range appXDS.prevServices {
			//fmt.Printf("%v \n", serviceKey)
			var ipList []k8s.Address
			for _, ip := range service.ClusterIPs {
				ipList = append(ipList, ip)
			}
			result += fmt.Sprintf(
				"\n>>>> Service %s.%s: IPs: %v \n",
				service.Namespace,
				service.Name,
				ipList,
			)

			for _, port := range service.Ports {
				result += fmt.Sprintf("%s %d -> %d   |   ", port.Protocol, port.Port, port.TargetPort)
			}
			result += "\n"

			for bsid, backendSet := range service.BackendsBySourceID {
				for _, backend := range backendSet.AddrSet {
					result += fmt.Sprintf("%v %v @ %d\n", bsid.EndpointSetName, backend, backendSet.Weight)
				}
			}
		}

		result += "\n\n"
	}

	return result
}

func (c *Cache) Get(appName, typeURL, clusterID string) (result grpc.Resource, ok bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	appXDS, ok := c.appXDSs[appName]
	if !ok {
		appXDS = NewAppXDS(c.logger, c.resOpts)
		c.appXDSs[appName] = appXDS
	}

	return appXDS.Get(typeURL, clusterID)
}

func getAppServices(nexus k8s.Nexus, services map[string]map[string]*k8s.Service) []*k8s.Service {
	var result []*k8s.Service
	for _, serviceKey := range nexus.Services {
		if serviceKey.IsWholeMesh() {
			result = []*k8s.Service{}
			for _, namespacedServices := range services {
				for _, service := range namespacedServices {
					result = append(result, service)
				}
			}
			return result
		}
		if serviceKey.IsNSRegexp() {
			for namespace, namespacedServices := range services {
				matched, _ := regexp.MatchString(serviceKey.Namespace, namespace)
				if !matched {
					continue
				}
				if !serviceKey.IsWholeNamespace() {
					service, ok := namespacedServices[serviceKey.Name]
					if !ok {
						continue
					}
					result = append(result, service)
					continue
				}
				for _, service := range namespacedServices {
					result = append(result, service)
				}
			}
			continue
		}
		if serviceKey.IsWholeNamespace() {
			for _, service := range services[serviceKey.Namespace] {
				result = append(result, service)
			}
			continue
		}

		service, ok := services[serviceKey.Namespace][serviceKey.Name]
		if !ok {
			continue
		}

		result = append(result, service)
	}

	return result
}
