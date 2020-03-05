package xds

import (
	"sync"

	"github.com/sirupsen/logrus"

	"github.com/avito-tech/navigator/pkg/grpc"
	"github.com/avito-tech/navigator/pkg/k8s"
	"github.com/avito-tech/navigator/pkg/xds/resources"
)

type ClusterResource interface {
	UpdateServices(updated, deleted []*k8s.Service)
	UpdateIngresses(updated map[k8s.QualifiedName]*k8s.Ingress)
	UpdateGateway(gw *k8s.Gateway)
	Get(clusterID string) (resource grpc.Resource, ok bool)
	TypeURL() string
	NotifySubscribers()
	String() string
}

type AppXDS struct {
	logger logrus.FieldLogger

	mu               sync.RWMutex
	clusterResources map[string]ClusterResource

	// prevServices used for debug handler
	prevServices map[k8s.QualifiedName]*k8s.Service
}

func NewAppXDS(logger logrus.FieldLogger, resOpts []resources.FuncOpt) *AppXDS {
	resourcesList := []ClusterResource{
		resources.NewClusterCache(resOpts...),
		resources.NewEndpointCache(resOpts...),
		resources.NewListenerCache(resOpts...),
		resources.NewRouteCache(resOpts...),
	}

	resourcesMap := map[string]ClusterResource{}
	for _, resource := range resourcesList {
		resourcesMap[resource.TypeURL()] = resource
	}

	return &AppXDS{
		prevServices:     make(map[k8s.QualifiedName]*k8s.Service),
		clusterResources: resourcesMap,
		logger:           logger.WithField("context", "AppXDS"),
	}
}

// Update is invoked in case service or endpoints kube entities have been updated;
// updates corresponding info in appXDS cache
func (x *AppXDS) Update(services []*k8s.Service) {
	x.mu.Lock()
	defer x.mu.Unlock()

	for _, resource := range x.clusterResources {
		resource.UpdateServices(servicesDiff(x.prevServices, services))
	}
	x.prevServices = servicesToState(services)
	for _, resource := range x.clusterResources {
		resource.NotifySubscribers()
	}
}

// IngressUpdate is invoked when kube ingress resource has been updated
func (x *AppXDS) IngressUpdate(ingresses map[k8s.QualifiedName]*k8s.Ingress) {
	x.mu.Lock()
	defer x.mu.Unlock()

	for _, resource := range x.clusterResources {
		resource.UpdateIngresses(ingresses)
	}

	// notify subscribed GRPC clients that resource updated
	for _, resource := range x.clusterResources {
		resource.NotifySubscribers()
	}
}

// GatewayUpdate is invoked when gateway resource has been updated
func (x *AppXDS) GatewayUpdate(gateway *k8s.Gateway) {
	x.mu.Lock()
	defer x.mu.Unlock()

	for _, resource := range x.clusterResources {
		resource.UpdateGateway(gateway)
	}

	// notify subscribed GRPC clients that resource updated
	for _, resource := range x.clusterResources {
		resource.NotifySubscribers()
	}
}

func (x *AppXDS) Get(typeURL, clusterID string) (res grpc.Resource, ok bool) {
	clusterResource, ok := x.clusterResources[typeURL]
	if !ok {
		return nil, false
	}

	return clusterResource.Get(clusterID)
}

func servicesDiff(oldServices map[k8s.QualifiedName]*k8s.Service, newServices []*k8s.Service) (updated, deleted []*k8s.Service) {

	for _, newService := range newServices {
		delete(oldServices, newService.Key())
		updated = append(updated, newService)
	}

	for _, oldService := range oldServices {
		deleted = append(deleted, oldService)
	}

	return updated, deleted
}

func servicesToState(services []*k8s.Service) map[k8s.QualifiedName]*k8s.Service {
	state := map[k8s.QualifiedName]*k8s.Service{}
	for _, service := range services {
		state[service.Key()] = service
	}
	return state
}
