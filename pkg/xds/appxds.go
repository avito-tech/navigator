package xds

import (
	"sync"

	"github.com/sirupsen/logrus"

	"github.com/avito-tech/navigator/pkg/grpc"
	"github.com/avito-tech/navigator/pkg/k8s"
	"github.com/avito-tech/navigator/pkg/xds/resources"
)

type ClusterResource interface {
	AddService(service *k8s.Service) error
	RemoveService(serviceKey k8s.QualifiedName) error
	Get(clusterID string) (resource grpc.Resource, ok bool)
	NotifySubscribers()
	TypeURL() string
}

type AppXDS struct {
	logger logrus.FieldLogger

	mu               sync.RWMutex
	services         map[k8s.QualifiedName]struct{}
	clusterResources map[string]ClusterResource
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
		services:         make(map[k8s.QualifiedName]struct{}),
		clusterResources: resourcesMap,
		logger:           logger.WithField("context", "AppXDS"),
	}
}

func (x *AppXDS) Update(services []*k8s.Service) {
	x.mu.Lock()
	defer x.mu.Unlock()

	oldServices := x.services
	x.services = make(map[k8s.QualifiedName]struct{})

	for _, service := range services {
		x.services[service.Key()] = struct{}{}
		delete(oldServices, service.Key())

		for typeURL, resource := range x.clusterResources {
			err := resource.AddService(service)
			if err != nil {
				x.logger.Errorf("Failed to add service %q to resource %q: %s", service.Key(), typeURL, err.Error())
			}
		}
	}

	// remove services, not exists in serviceList and nexus
	for serviceKey := range oldServices {
		for typeURL, resource := range x.clusterResources {
			err := resource.RemoveService(serviceKey)
			if err != nil {
				x.logger.Errorf("Failed to remove service %q from resource %q: %s", serviceKey, typeURL, err.Error())
			}
		}
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
