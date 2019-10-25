package k8s

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/sirupsen/logrus"

	"sync"
)

type Cache interface {
	UpdateService(namespace, name, clusterID, ClusterIP string, ports []Port) (updatedServiceKeys []QualifiedName)
	UpdateBackends(serviceName QualifiedName, clusterID string, endpointSetName QualifiedName, weight int, ips []string) (updatedServiceKeys []QualifiedName)
	RemoveService(namespace, name string, clusterID string) (updatedServiceKeys []QualifiedName)
	RemoveBackends(serviceName QualifiedName, clusterID string, endpointSetName QualifiedName) (updatedServiceKeys []QualifiedName)
	FlushServiceByClusterID(serviceName QualifiedName, clusterID string)
	GetSnapshot() (services map[string]map[string]*Service)
}

var prometheusServiceCount = promauto.NewGauge(prometheus.GaugeOpts{
	Name: "navigator_k8s_cache_services_count",
	Help: "Count of services, stored in k8s cache",
})

var prometheusBackendsCount = promauto.NewGaugeVec(prometheus.GaugeOpts{
	Name: "navigator_k8s_cache_backends_count",
	Help: "Count of backends by service, stored in k8s cache",
}, []string{"service", "clusterID"})

type cache struct {
	mu       sync.RWMutex
	services map[QualifiedName]*Service
	logger   logrus.FieldLogger
}

func NewCache(logger logrus.FieldLogger) Cache {
	return &cache{
		services: make(map[QualifiedName]*Service),
		logger:   logger.WithField("context", "k8s.cache"),
	}
}

func (c *cache) UpdateService(namespace, name, clusterID, clusterIP string, ports []Port) (updatedServiceKeys []QualifiedName) {
	c.mu.Lock()
	defer c.mu.Unlock()

	key := NewQualifiedName(namespace, name)
	service, ok := c.services[key]
	if !ok {
		service = NewService(namespace, name)
		c.services[key] = service
		prometheusServiceCount.Inc()
	}

	updated := service.UpdateClusterIP(clusterID, clusterIP)
	updated = service.UpdatePorts(ports) || updated

	if updated {
		return []QualifiedName{key}
	}
	return nil
}

func (c *cache) UpdateBackends(serviceName QualifiedName, clusterID string, endpointSetName QualifiedName, weight int, ips []string) (updatedServiceKeys []QualifiedName) {
	c.mu.Lock()
	defer c.mu.Unlock()

	service, ok := c.services[serviceName]
	if !ok {
		service = NewService(serviceName.Namespace, serviceName.Name)
		c.services[serviceName] = service
		prometheusServiceCount.Inc()
	}

	bsid := BackendSourceID{ClusterID: clusterID, EndpointSetName: endpointSetName}
	oldLen := len(service.BackendsBySourceID[bsid].AddrSet)

	updated := service.UpdateBackends(clusterID, endpointSetName, weight, ips)
	prometheusBackendsCount.With(prometheus.Labels{"service": serviceName.String(), "clusterID": clusterID}).Add(
		float64(len(service.BackendsBySourceID[bsid].AddrSet) - oldLen),
	)

	if updated {
		return []QualifiedName{serviceName}
	}

	return nil
}

func (c *cache) RemoveService(namespace, name string, clusterID string) (updatedServiceKeys []QualifiedName) {
	c.mu.Lock()
	defer c.mu.Unlock()

	key := NewQualifiedName(namespace, name)

	svc, ok := c.services[key]
	if !ok {
		return nil
	}
	svc.DeleteClusterIP(clusterID)
	prometheusBackendsCount.Delete(prometheus.Labels{"service": key.String(), "clusterID": clusterID})

	if len(svc.ClusterIPs) == 0 {
		delete(c.services, key)
		prometheusServiceCount.Dec()
	}

	return []QualifiedName{key}
}

func (c *cache) RemoveBackends(serviceName QualifiedName, clusterID string, endpointSetName QualifiedName) (updatedServiceKeys []QualifiedName) {
	c.mu.Lock()
	defer c.mu.Unlock()

	service, ok := c.services[serviceName]
	if !ok {
		return nil
	}

	bsid := BackendSourceID{ClusterID: clusterID, EndpointSetName: endpointSetName}
	oldLen := len(service.BackendsBySourceID[bsid].AddrSet)

	updated := service.DeleteBackends(clusterID, endpointSetName)

	prometheusBackendsCount.With(prometheus.Labels{"service": serviceName.String(), "clusterID": clusterID}).Add(
		float64(len(service.BackendsBySourceID[bsid].AddrSet) - oldLen),
	)

	if updated {
		return []QualifiedName{serviceName}
	}

	return nil
}

// GetSnapshot returns services indexed by namespace and name: services[namespace][name]
func (c *cache) GetSnapshot() (services map[string]map[string]*Service) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	services = make(map[string]map[string]*Service)

	for i, service := range c.services {
		if _, ok := services[i.Namespace]; !ok {
			services[i.Namespace] = make(map[string]*Service)
		}
		services[i.Namespace][i.Name] = service.Copy()
	}

	return
}

func (c *cache) FlushServiceByClusterID(serviceName QualifiedName, clusterID string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	service, ok := c.services[serviceName]
	if !ok {
		return
	}

	for backend := range service.BackendsBySourceID {
		if backend.ClusterID == clusterID {
			delete(service.BackendsBySourceID, backend)
		}
	}
}
