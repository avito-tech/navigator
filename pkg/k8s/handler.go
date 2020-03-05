package k8s

import (
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/sirupsen/logrus"
	v1 "k8s.io/api/core/v1"
	"k8s.io/api/extensions/v1beta1"
	listersCoreV1 "k8s.io/client-go/listers/core/v1"
	k8sCache "k8s.io/client-go/tools/cache"

	navigatorV1 "github.com/avito-tech/navigator/pkg/apis/navigator/v1"
)

var (
	eventsCount = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "navigator_informer_events_total",
		Help: "Current amount of k8s events, received by informers",
	}, []string{"action", "informer", "clusterID"})

	renewServiceErrorsCount = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "navigator_canary_service_renew_errors_total",
		Help: "Current amount of errors during canary service renewal",
	}, []string{"clusterID"})
)

type serviceNotifier interface {
	NotifyServicesUpdated(updatedServiceKeys []QualifiedName)
}

type nexusNotifier interface {
	NotifyNexusesUpdated(updatedAppNames []string, isGateway bool)
}

type ingressNotifier interface {
	NotifyIngressUpdated(updatedIngress *Ingress)
}

type gatewayNotifier interface {
	NotifyGatewayUpdated(updatedGateway *Gateway, deleted bool)
}

type EndpointEventHandler struct {
	logger          logrus.FieldLogger
	cache           Cache
	serviceNotifier serviceNotifier
	canary          Canary
	clusterID       string
	mu              sync.Mutex
	eventsProcessed int32
}

func NewEndpointEventHandler(logger logrus.FieldLogger, cache Cache, serviceNotifier serviceNotifier, canary Canary, clusterID string) *EndpointEventHandler {
	return &EndpointEventHandler{
		clusterID:       clusterID,
		logger:          logger.WithField("context", "k8s.EndpointEventHandler"),
		cache:           cache,
		canary:          canary,
		serviceNotifier: serviceNotifier,
	}
}

func (h *EndpointEventHandler) EventsCount() int32 {
	return atomic.LoadInt32(&h.eventsProcessed)
}

func (h *EndpointEventHandler) OnAdd(obj interface{}) {
	h.logger.WithField("ClusterID", h.clusterID).Tracef("EndpointEventHandler.OnAdd")
	eventsCount.With(map[string]string{"action": "add", "informer": "endpoints", "clusterID": h.clusterID}).Inc()
	h.handleEndpoint(obj, false)
}

func (h *EndpointEventHandler) OnUpdate(_, newObj interface{}) {
	h.logger.WithField("ClusterID", h.clusterID).Tracef("EndpointEventHandler.OnUpdate")
	eventsCount.With(map[string]string{"action": "update", "informer": "endpoints", "clusterID": h.clusterID}).Inc()
	h.handleEndpoint(newObj, false)
}

func (h *EndpointEventHandler) OnDelete(obj interface{}) {
	h.logger.WithField("ClusterID", h.clusterID).Tracef("EndpointEventHandler.OnDelete")
	eventsCount.With(map[string]string{"action": "delete", "informer": "endpoints", "clusterID": h.clusterID}).Inc()
	h.handleEndpoint(obj, true)
}

func (h *EndpointEventHandler) handleEndpoint(obj interface{}, delete bool) {

	var ep *v1.Endpoints
	switch event := obj.(type) {
	case k8sCache.DeletedFinalStateUnknown:
		h.handleEndpoint(event.Obj, delete)
		return
	case *v1.Endpoints:
		ep = event
	default:
		h.logger.Warningf("Trying to handle not endpoint: %#v", obj)
		return
	}

	defer atomic.AddInt32(&h.eventsProcessed, 1)
	h.logger.WithField("ClusterID", h.clusterID).Debugf("Handling Endpoints %s.%s", ep.Namespace, ep.Name)

	h.mu.Lock()
	defer h.mu.Unlock()

	updatedSvcs := h.doHandleEndpoint(ep, delete)

	h.notify(updatedSvcs)
	h.logger.WithField("ClusterID", h.clusterID).Debugf("Endpoint %s.%s updated: %v", ep.Namespace, ep.Name, updatedSvcs)
}

func (h *EndpointEventHandler) doHandleEndpoint(ep *v1.Endpoints, delete bool) (updatedSvcs []QualifiedName) {
	var ips []string
	for _, subset := range ep.Subsets {
		for _, address := range subset.Addresses {
			ips = append(ips, address.IP)
		}
	}

	for _, mapping := range h.canary.GetMappings(NewQualifiedName(ep.Namespace, ep.Name), h.clusterID) {
		h.logger.WithField("cluster", h.clusterID).Tracef("doHandleEndpoint handling mapping %+v for ep %s.%s", mapping, ep.Namespace, ep.Name)
		var u []QualifiedName
		if delete {
			u = h.cache.RemoveBackends(mapping.ServiceName, h.clusterID, mapping.EndpointSetName)
		} else {
			u = h.cache.UpdateBackends(mapping.ServiceName, h.clusterID, mapping.EndpointSetName, mapping.Weight, ips)
		}

		updatedSvcs = append(updatedSvcs, u...)
	}

	return updatedSvcs
}

func (h *EndpointEventHandler) notify(updatedServiceKeys []QualifiedName) {
	if len(updatedServiceKeys) == 0 {
		return
	}
	go h.serviceNotifier.NotifyServicesUpdated(updatedServiceKeys)
}

func (h *EndpointEventHandler) Lock() {
	h.mu.Lock()
}

func (h *EndpointEventHandler) Unlock() {
	h.mu.Unlock()
}

type ServiceEventHandler struct {
	logger          logrus.FieldLogger
	cache           Cache
	ingressCache    IngressCache
	serviceNotifier serviceNotifier
	ingressNotifier ingressNotifier
	clusterID       string
	eventsProcessed int32
}

func NewServiceEventHandler(
	logger logrus.FieldLogger,
	cache Cache,
	ingressCache IngressCache,
	serviceNotifier serviceNotifier,
	ingressNotifier ingressNotifier,
	clusterID string,
) *ServiceEventHandler {
	return &ServiceEventHandler{
		clusterID:       clusterID,
		logger:          logger.WithField("context", "k8s.ServiceEventHandler"),
		cache:           cache,
		ingressCache:    ingressCache,
		serviceNotifier: serviceNotifier,
		ingressNotifier: ingressNotifier,
	}
}

func (h *ServiceEventHandler) EventsCount() int32 {
	return atomic.LoadInt32(&h.eventsProcessed)
}

func (h *ServiceEventHandler) OnAdd(obj interface{}) {
	h.logger.WithField("ClusterID", h.clusterID).Tracef("EndpointEventHandler.OnAdd")
	eventsCount.With(map[string]string{"action": "add", "informer": "services", "clusterID": h.clusterID}).Inc()
	h.handleService(obj, false)
}

func (h *ServiceEventHandler) OnUpdate(_, newObj interface{}) {
	h.logger.WithField("ClusterID", h.clusterID).Tracef("EndpointEventHandler.OnUpdate")
	eventsCount.With(map[string]string{"action": "update", "informer": "services", "clusterID": h.clusterID}).Inc()
	h.handleService(newObj, false)
}

func (h *ServiceEventHandler) OnDelete(obj interface{}) {
	h.logger.WithField("ClusterID", h.clusterID).Tracef("EndpointEventHandler.OnDelete")
	eventsCount.With(map[string]string{"action": "delete", "informer": "services", "clusterID": h.clusterID}).Inc()
	h.handleService(obj, true)
}

func (h *ServiceEventHandler) handleService(obj interface{}, delete bool) {

	var svc *v1.Service
	switch event := obj.(type) {
	case k8sCache.DeletedFinalStateUnknown:
		h.handleService(event.Obj, delete)
		return
	case *v1.Service:
		svc = event
	default:
		h.logger.Warningf("Trying to handle not service: %#v", obj)
		return
	}

	defer atomic.AddInt32(&h.eventsProcessed, 1)
	h.logger.WithField("ClusterID", h.clusterID).Tracef("Handling Service %s.%s", svc.Namespace, svc.Name)

	if svc.Spec.Type == v1.ServiceTypeExternalName {
		return
	}

	if delete {
		result := h.cache.RemoveService(svc.Namespace, svc.Name, h.clusterID)
		h.notify(result)
		h.logger.Debugf("Service %s.%s  removed: %v", svc.Namespace, svc.Name, result)
		return
	}

	var ports []Port
	for _, p := range svc.Spec.Ports {
		port := Port{
			Port:       int(p.Port),
			Protocol:   Protocol(p.Protocol),
			TargetPort: int(p.TargetPort.IntVal),
			Name:       p.Name,
		}

		ports = append(ports, port)
	}

	result := h.cache.UpdateService(svc.Namespace, svc.Name, h.clusterID, svc.Spec.ClusterIP, ports)
	updatedIngresses := h.ingressCache.UpdateServicePort(h.clusterID, svc.Namespace, svc.Name, ports)
	h.notify(result)
	for _, updatedIngress := range updatedIngresses {
		// we pass here equal updatedIngress cos we have only port replacement (xds resource replacement based only on host and location)
		h.ingressNotifier.NotifyIngressUpdated(updatedIngress)
	}
	h.logger.WithField("ClusterID", h.clusterID).Tracef("Service %s.%s  updated: %v", svc.Namespace, svc.Name, result)
}

func (h *ServiceEventHandler) notify(updatedServiceKeys []QualifiedName) {
	if len(updatedServiceKeys) == 0 {
		return
	}
	go h.serviceNotifier.NotifyServicesUpdated(updatedServiceKeys)
}

type CanaryReleaseEventHandler struct {
	logger          logrus.FieldLogger
	canary          Canary
	cache           Cache
	epLister        listersCoreV1.EndpointsLister
	clusterID       string
	epHandler       *EndpointEventHandler
	eventsProcessed int32
}

func NewCanaryReleaseEventHandler(logger logrus.FieldLogger, cache Cache, canary Canary, clusterID string, epLister listersCoreV1.EndpointsLister, epHandler *EndpointEventHandler) *CanaryReleaseEventHandler {
	return &CanaryReleaseEventHandler{
		logger:    logger.WithField("context", "k8s.CanaryReleaseEventHandler"),
		canary:    canary,
		cache:     cache,
		epLister:  epLister,
		clusterID: clusterID,
		epHandler: epHandler,
	}
}

func (h *CanaryReleaseEventHandler) EventsCount() int32 {
	return atomic.LoadInt32(&h.eventsProcessed)
}

func (h *CanaryReleaseEventHandler) OnAdd(obj interface{}) {
	h.logger.WithField("ClusterID", h.clusterID).Tracef("CanaryReleaseEventHandler.OnAdd")
	eventsCount.With(map[string]string{"action": "add", "informer": "canary_releases", "clusterID": h.clusterID}).Inc()
	h.handleCanaryRelease(obj, false)
}

func (h *CanaryReleaseEventHandler) OnUpdate(oldObj, newObj interface{}) {
	h.logger.WithField("ClusterID", h.clusterID).Tracef("CanaryReleaseEventHandler.OnUpdate")
	eventsCount.With(map[string]string{"action": "update", "informer": "canary_releases", "clusterID": h.clusterID}).Inc()
	h.handleCanaryRelease(newObj, false)
}

func (h *CanaryReleaseEventHandler) OnDelete(obj interface{}) {
	h.logger.WithField("ClusterID", h.clusterID).Tracef("CanaryReleaseEventHandler.OnDelete")
	eventsCount.With(map[string]string{"action": "delete", "informer": "canary_releases", "clusterID": h.clusterID}).Inc()
	h.handleCanaryRelease(obj, true)
}

func (h *CanaryReleaseEventHandler) handleCanaryRelease(obj interface{}, delete bool) {

	var cr *navigatorV1.CanaryRelease
	switch event := obj.(type) {
	case k8sCache.DeletedFinalStateUnknown:
		h.handleCanaryRelease(event.Obj, delete)
		return
	case *navigatorV1.CanaryRelease:
		cr = event
	default:
		h.logger.Warningf("Trying to handle not CanaryRelease: %#v", obj)
		return
	}

	defer atomic.AddInt32(&h.eventsProcessed, 1)
	h.logger.WithField("ClusterID", h.clusterID).Tracef("Handling CanaryRelease %s.%s", cr.Namespace, cr.Name)

	serviceName := NewQualifiedName(cr.Namespace, cr.Name)

	if delete {
		h.canary.DeleteMapping(serviceName, h.clusterID)
		// explicitly add default mapping to lnk default endpoints with service again
		h.renewService(
			serviceName,
			[]EndpointMapping{h.canary.GetDefaultMapping(serviceName)},
		)
		return
	}

	var mappings []EndpointMapping
	for _, rr := range cr.Spec.Backends {
		mapping := NewEndpointMapping(
			NewQualifiedName(rr.Namespace, rr.Name),
			serviceName,
			rr.Weight,
		)
		mappings = append(mappings, mapping)
	}

	h.canary.UpdateMapping(serviceName, h.clusterID, mappings)

	h.renewService(serviceName, mappings)
}

func (h *CanaryReleaseEventHandler) renewService(serviceName QualifiedName, newMappings []EndpointMapping) {
	h.epHandler.Lock()
	defer h.epHandler.Unlock()

	// non-atomic Flush isn't bad due to newly connected envoy will receive its endpoints soon
	// and already connected envoys will be notified only after adding new endpoints
	h.cache.FlushServiceByClusterID(serviceName, h.clusterID)

	var updatedSvcs []QualifiedName
	h.logger.Debugf("RENEW: listing EPS name in cluster %q mapping: %+v", h.clusterID, newMappings)
	for _, mapping := range newMappings {
		eps, err := h.epLister.Endpoints(mapping.EndpointSetName.Namespace).Get(mapping.EndpointSetName.Name)
		if err != nil {
			h.logger.Errorf("failed to get endpoints %s from lister:", mapping.EndpointSetName)
			renewServiceErrorsCount.With(map[string]string{"clusterID": h.clusterID})
			continue
		}
		updatedSvcs = append(
			updatedSvcs,
			h.epHandler.doHandleEndpoint(eps, false)...,
		)
	}

	h.logger.Debugf("RENEW: finished in cluster %q", h.clusterID)
	h.epHandler.notify(updatedSvcs)
}

// NexusEventHandler handles nexus updates
type NexusEventHandler struct {
	logger          logrus.FieldLogger
	nexus           NexusCache
	nexusNotifier   nexusNotifier
	clusterID       string
	eventsProcessed int32
}

func NewNexusEventHandler(
	logger logrus.FieldLogger,
	nexusCache NexusCache,
	nexusNotifier nexusNotifier,
	clusterID string,
) *NexusEventHandler {
	return &NexusEventHandler{
		logger:        logger.WithField("context", "k8s.NexusHandler"),
		nexus:         nexusCache,
		nexusNotifier: nexusNotifier,
		clusterID:     clusterID,
	}
}

func (h *NexusEventHandler) EventsCount() int32 {
	return atomic.LoadInt32(&h.eventsProcessed)
}

func (h *NexusEventHandler) OnAdd(obj interface{}) {
	h.logger.WithField("ClusterID", h.clusterID).Tracef("CanaryReleaseEventHandler.OnUpdate")
	h.handleNexusEvent(obj, false)
}

func (h *NexusEventHandler) OnUpdate(oldObj, newObj interface{}) {
	h.logger.WithField("ClusterID", h.clusterID).Tracef("CanaryReleaseEventHandler.OnUpdate")
	h.handleNexusEvent(newObj, false)
}

func (h *NexusEventHandler) OnDelete(obj interface{}) {
	h.logger.WithField("ClusterID", h.clusterID).Tracef("CanaryReleaseEventHandler.OnUpdate")
	h.handleNexusEvent(obj, true)
}

func (h *NexusEventHandler) handleNexusEvent(obj interface{}, delete bool) {
	dep, err := h.getK8sNexus(obj)
	if err != nil {
		h.logger.WithError(err).Warn("cannot get nexus data")
		return
	}

	defer atomic.AddInt32(&h.eventsProcessed, 1)
	h.logger.WithField("ClusterID", h.clusterID).Tracef("Handling Nexus %s.%s", dep.Namespace, dep.Name)

	var updatedAppNames []string
	// We use dep.name as "partition ID" to guarantee that version is unique and permanent across k8s nexuses
	if delete {
		updatedAppNames = h.nexus.Delete(dep.Spec.AppName, dep.Name)
	} else {
		services := h.getNexusServices(dep)
		updatedAppNames = h.nexus.Update(dep.Spec.AppName, dep.Name, services)
	}

	h.nexusNotifier.NotifyNexusesUpdated(updatedAppNames, false)

	h.logger.WithField("ClusterID", h.clusterID).Tracef("Nexus %s.%s  updated: %v", dep.Namespace, dep.Name, updatedAppNames)
}

func (h *NexusEventHandler) getK8sNexus(obj interface{}) (*navigatorV1.Nexus, error) {
	switch dep := obj.(type) {
	case k8sCache.DeletedFinalStateUnknown:
		return h.getK8sNexus(dep.Obj)
	case *navigatorV1.Nexus:
		return dep, nil
	default:
		return nil, fmt.Errorf("unknown nexus data: %#v", obj)
	}
}

func (h *NexusEventHandler) getNexusServices(dep *navigatorV1.Nexus) []QualifiedName {
	services := make([]QualifiedName, 0, len(dep.Spec.Services))
	for _, service := range dep.Spec.Services {
		services = append(services, NewQualifiedName(service.Namespace, service.Name))
	}
	return services
}

type IngressEventHandler struct {
	logger          logrus.FieldLogger
	nexusCache      NexusCache
	ingressCache    IngressCache
	gatewayCache    GatewayCache
	k8sCache        Cache
	ingressNotifier ingressNotifier
	nexusNotifier   nexusNotifier
	clusterID       string
	eventsProcessed int32
}

func NewIngressEventHandler(logger logrus.FieldLogger,
	nexusCache NexusCache,
	ingressCache IngressCache,
	gatewayCache GatewayCache,
	k8sCache Cache,
	ingressNotifier ingressNotifier,
	nexusNotifier nexusNotifier,
	clusterID string,
) *IngressEventHandler {
	return &IngressEventHandler{
		clusterID:       clusterID,
		logger:          logger.WithField("context", "k8s.IngressEventHandler"),
		nexusCache:      nexusCache,
		ingressCache:    ingressCache,
		gatewayCache:    gatewayCache,
		k8sCache:        k8sCache,
		ingressNotifier: ingressNotifier,
		nexusNotifier:   nexusNotifier,
	}
}

func (h *IngressEventHandler) EventsCount() int32 {
	return atomic.LoadInt32(&h.eventsProcessed)
}

func (h *IngressEventHandler) OnAdd(obj interface{}) {
	h.logger.WithField("ClusterID", h.clusterID).Tracef("IngressEventHandler.OnAdd")
	eventsCount.With(map[string]string{"action": "add", "informer": "services", "clusterID": h.clusterID}).Inc()
	h.handleIngress(obj, false)
}

func (h *IngressEventHandler) OnUpdate(_, newObj interface{}) {
	h.logger.WithField("ClusterID", h.clusterID).Tracef("IngressEventHandler.OnUpdate")
	eventsCount.With(map[string]string{"action": "update", "informer": "services", "clusterID": h.clusterID}).Inc()
	h.handleIngress(newObj, false)
}

func (h *IngressEventHandler) OnDelete(obj interface{}) {
	h.logger.WithField("ClusterID", h.clusterID).Tracef("IngressEventHandler.OnDelete")
	eventsCount.With(map[string]string{"action": "delete", "informer": "services", "clusterID": h.clusterID}).Inc()
	h.handleIngress(obj, true)
}

func (h *IngressEventHandler) handleIngress(obj interface{}, delete bool) {
	var ingress *v1beta1.Ingress
	switch event := obj.(type) {
	case k8sCache.DeletedFinalStateUnknown:
		h.handleIngress(event.Obj, delete)
		return
	case *v1beta1.Ingress:
		ingress = event
	default:
		h.logger.Warningf("Trying to handle not Ingress: %#v", obj)
		return
	}

	defer atomic.AddInt32(&h.eventsProcessed, 1)
	h.logger.WithField("ClusterID", h.clusterID).Tracef("Handling Ingress %s.%s", ingress.Namespace, ingress.Name)

	if delete {
		ing := h.ingressCache.Remove(h.clusterID, ingress)
		h.ingressNotifier.NotifyIngressUpdated(ing)
		return
	}

	svcs := h.k8sCache.GetSnapshot()
	ports := h.getPortsForIngress(ingress, svcs)

	ing := h.ingressCache.Update(h.clusterID, ingress, ports)

	clusterIDs := h.ingressCache.GetClusterIDsForIngress(
		NewQualifiedName(ingress.Namespace, ingress.Name), ing.Class)

	clusterExists := false
	for _, cID := range clusterIDs {
		if cID == ing.ClusterID {
			clusterExists = true
		}
	}
	if !clusterExists {
		clusterIDs = append(clusterIDs, ing.ClusterID)
	}

	var updatedApps []string
	for _, clusterID := range clusterIDs {
		gateways := h.gatewayCache.GatewaysByClass(clusterID, ing.Class)
		for _, gw := range gateways {
			services := h.ingressCache.GetServicesByClass(clusterID, ing.Class)
			updated := h.nexusCache.Update(gw.Name, gw.Name, services)
			updatedApps = append(updatedApps, updated...)
		}
	}

	h.nexusNotifier.NotifyNexusesUpdated(updatedApps, true)
	h.ingressNotifier.NotifyIngressUpdated(ing)
}

// getPortsForIngress returns service -> port -> pot name mapping
func (h *IngressEventHandler) getPortsForIngress(ingress *v1beta1.Ingress, services map[string]map[string]*Service) map[string]map[int]string {
	ports := make(map[string]map[int]string)
	if _, ok := services[ingress.Namespace]; !ok {
		return ports
	}
	for _, rule := range ingress.Spec.Rules {
		if rule.HTTP != nil {
			for _, path := range rule.HTTP.Paths {
				if svc, ok := services[ingress.Namespace][path.Backend.ServiceName]; ok {
					for _, port := range svc.Ports {
						if _, ok := ports[path.Backend.ServiceName]; !ok {
							ports[path.Backend.ServiceName] = make(map[int]string)
						}
						if int32(port.Port) == path.Backend.ServicePort.IntVal {
							ports[path.Backend.ServiceName][port.Port] = port.Name
						}
					}
				}
			}
		}
	}

	return ports
}

type GatewayEventHandler struct {
	logger          logrus.FieldLogger
	ingressCache    IngressCache
	nexusCache      NexusCache
	gatewayCache    GatewayCache
	nexusNotifier   nexusNotifier
	gatewayNotifier gatewayNotifier
	clusterID       string
	eventsProcessed int32
}

func NewGatewayEventHandler(
	logger logrus.FieldLogger,
	ingressCache IngressCache,
	nexusCache NexusCache,
	gatewayCache GatewayCache,
	nexusNotifier nexusNotifier,
	gatewayNotifier gatewayNotifier,
	clusterID string,
) *GatewayEventHandler {

	return &GatewayEventHandler{
		clusterID:       clusterID,
		logger:          logger.WithField("context", "k8s.GatewayEventHandler"),
		ingressCache:    ingressCache,
		nexusCache:      nexusCache,
		gatewayCache:    gatewayCache,
		nexusNotifier:   nexusNotifier,
		gatewayNotifier: gatewayNotifier,
	}
}

func (h *GatewayEventHandler) EventsCount() int32 {
	return atomic.LoadInt32(&h.eventsProcessed)
}

func (h *GatewayEventHandler) OnAdd(obj interface{}) {
	h.logger.WithField("ClusterID", h.clusterID).Tracef("EndpointEventHandler.OnAdd")
	eventsCount.With(map[string]string{"action": "add", "informer": "gateway", "clusterID": h.clusterID}).Inc()
	h.handleGatewayEvent(obj, false)
}

func (h *GatewayEventHandler) OnUpdate(_, newObj interface{}) {
	h.logger.WithField("ClusterID", h.clusterID).Tracef("EndpointEventHandler.OnUpdate")
	eventsCount.With(map[string]string{"action": "update", "informer": "gateway", "clusterID": h.clusterID}).Inc()
	h.handleGatewayEvent(newObj, false)
}

func (h *GatewayEventHandler) OnDelete(obj interface{}) {
	h.logger.WithField("ClusterID", h.clusterID).Tracef("EndpointEventHandler.OnDelete")
	eventsCount.With(map[string]string{"action": "delete", "informer": "gateway", "clusterID": h.clusterID}).Inc()
	h.handleGatewayEvent(obj, true)
}

func (h *GatewayEventHandler) handleGatewayEvent(obj interface{}, delete bool) {
	gw, err := h.getK8sGateway(obj)
	if err != nil {
		h.logger.WithError(err).Warn("cannot get nexus data")
		return
	}

	defer atomic.AddInt32(&h.eventsProcessed, 1)
	h.logger.WithField("ClusterID", h.clusterID).Tracef("Handling Gateway %s.%s", gw.Namespace, gw.Name)

	var nexusUpdated []string
	var gateway *Gateway

	// We use gw.name as "partition ID" to guarantee that version is unique and permanent across k8s nexuses
	if delete {
		nexusUpdated = h.nexusCache.Delete(gw.Name, gw.Name)
		gateway = h.gatewayCache.Delete(h.clusterID, gw)
		h.gatewayNotifier.NotifyGatewayUpdated(gateway, true)
	} else {
		services := h.ingressCache.GetServicesByClass(h.clusterID, gw.Spec.IngressClass)
		nexusUpdated = h.nexusCache.Update(gw.Name, gw.Name, services)

		gateway = h.gatewayCache.Update(h.clusterID, gw)
		h.gatewayNotifier.NotifyGatewayUpdated(gateway, false)
	}
	h.nexusNotifier.NotifyNexusesUpdated(nexusUpdated, true)

	h.logger.WithField("ClusterID", h.clusterID).Tracef("Nexus %s.%s  updated: %v", gw.Namespace, gw.Name, nexusUpdated)
	h.logger.WithField("ClusterID", h.clusterID).Tracef("Gateway %s.%s  updated: %s", gw.Namespace, gw.Name, gateway.Name)
}

func (h *GatewayEventHandler) getK8sGateway(obj interface{}) (*navigatorV1.Gateway, error) {
	switch dep := obj.(type) {
	case k8sCache.DeletedFinalStateUnknown:
		return h.getK8sGateway(dep.Obj)
	case *navigatorV1.Gateway:
		return dep, nil
	default:
		return nil, fmt.Errorf("unknown nexus data: %#v", obj)
	}
}
