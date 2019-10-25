package k8s

import (
	"sync"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

const (
	EndpointsWeightSumForCluster = 1000 // weight sum for one endpoint set for particular cluster
)

var (
	prometheusCanariesCount = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "navigator_canary_cache_canaries_count",
		Help: "Count of canaried services",
	})
)

type EndpointMapping struct {
	EndpointSetName, ServiceName QualifiedName
	Weight                       int
}

func NewEndpointMapping(endpointSetName, serviceName QualifiedName, weight int) EndpointMapping {
	return EndpointMapping{
		EndpointSetName: endpointSetName,
		ServiceName:     serviceName,
		Weight:          weight,
	}
}

type Canary interface {
	GetMappings(EndpointName QualifiedName, clusterID string) map[QualifiedName]EndpointMapping
	UpdateMapping(ServiceName QualifiedName, clusterID string, mappings []EndpointMapping)
	DeleteMapping(ServiceName QualifiedName, clusterID string)
	GetDefaultMapping(Name QualifiedName) EndpointMapping
}

type canary struct {
	// EndpointsToServices contains the mapping from EndpointSetName (indexed by cluster) name to service name
	// map[EndpointsName by cluster]map[ServiceName]EndpointMapping
	EndpointsToServices map[BackendSourceID]map[QualifiedName]EndpointMapping
	// currentCanariesCount contains current canaries CR count for this canary struct due to
	// metrics navigator_canary_cache_canaries_count is shared across all instances of class canary
	// and we must use inc/dec with diff on the metrics to maintain correct sum of all canaries across all canary instances
	currentCanariesCount int
	mu                   sync.RWMutex
}

func NewCanary() Canary {
	return &canary{
		EndpointsToServices: make(map[BackendSourceID]map[QualifiedName]EndpointMapping),
	}
}

func (c *canary) GetDefaultMapping(name QualifiedName) EndpointMapping {
	return EndpointMapping{EndpointSetName: name, ServiceName: name, Weight: EndpointsWeightSumForCluster}
}

// GetMappings returns Mapping shows which SERVICES linked with ENDPOINT named  `endpointSetName`
// 1. IF there is no CANARY, that points to ENDPOINT named `endpointSetName`,
//    THEN ENDPOINT linked only with default service aka `DefaultEpService` (service.name = endpointSetName)
//    RETURN default mapping aka `DefaultMapping` (service.name = endpointSetName)
// 2. IF CANARIES points to ENDPOINT `endpointSetName` exists,
//    THEN ENDPOINT linked with services of this CANARIES and with `DefaultEpService`
//    RETURN mapping to services of CANARIES and `DefaultEpService`
// 3. IF CANARIES points to ENDPOINT `endpointSetName` exists AND there is canary for `DefaultEpService` AKA DefaultEpServiceCanary (canary.service = endpointSetName)
//    THEN don't use `DefaultMapping` due to DefaultEpServiceCanary EXPLICITLY defines relations between `DefaultEpService` and its endpoint
//    RETURN mapping to services of CANARIES including DefaultEpServiceCanary
func (c *canary) GetMappings(endpointSetName QualifiedName, clusterID string) map[QualifiedName]EndpointMapping {
	c.mu.RLock()
	defer c.mu.RUnlock()

	// First of all, initialize result with default mapping
	var exists bool
	for bsid, servicesMapping := range c.EndpointsToServices {
		if bsid.ClusterID == clusterID {
			_, ok := servicesMapping[endpointSetName]
			exists = exists || ok
		}
	}
	result := map[QualifiedName]EndpointMapping{}
	if !exists {
		result[endpointSetName] = c.GetDefaultMapping(endpointSetName)
	}

	// then, trying to find explicitly defined mapping.
	// if such mapping found, replace default mapping to this service with explicit
	for _, w := range c.EndpointsToServices[NewBackendSourceID(endpointSetName, clusterID)] {
		result[w.ServiceName] = w
	}

	return result
}

func (c *canary) UpdateMapping(serviceName QualifiedName, clusterID string, mappings []EndpointMapping) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// first of all, remove any existing mappings TO specified service in particular cluster
	for backendSourceID, epmSet := range c.EndpointsToServices {
		if backendSourceID.ClusterID == clusterID {
			delete(epmSet, serviceName)
		}
	}

	// second, add new mappings to specified service
	for _, mapping := range mappings {
		backendSourceID := NewBackendSourceID(mapping.EndpointSetName, clusterID)
		// just ensure, that mappings maps to specified service to avoid really hard to find bugs
		mapping.ServiceName = serviceName
		if _, ok := c.EndpointsToServices[backendSourceID]; !ok {
			c.EndpointsToServices[backendSourceID] = map[QualifiedName]EndpointMapping{}
		}
		c.EndpointsToServices[backendSourceID][mapping.ServiceName] = mapping
	}

	c.updateMetrics()
}

func (c *canary) DeleteMapping(serviceName QualifiedName, clusterID string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// remove any existing mappings TO specified service in particular cluster
	for backendSourceID, epmSet := range c.EndpointsToServices {
		if backendSourceID.ClusterID == clusterID {
			delete(epmSet, serviceName)
		}
	}

	c.updateMetrics()
}

func (c *canary) updateMetrics() {
	canariedServices := map[QualifiedName]bool{}
	for _, epmSet := range c.EndpointsToServices {
		for serviceName := range epmSet {
			canariedServices[serviceName] = true
		}
	}

	c.currentCanariesCount = len(canariedServices)
	prometheusCanariesCount.Set(float64(c.currentCanariesCount))
}
