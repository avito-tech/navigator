// +build unit

package k8s

import (
	"io/ioutil"
	"testing"
	"time"

	navigatorV1 "github.com/avito-tech/navigator/pkg/apis/navigator/v1"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"k8s.io/api/core/v1"
	"k8s.io/api/extensions/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/informers"
	fakekube "k8s.io/client-go/kubernetes/fake"
	k8sCache "k8s.io/client-go/tools/cache"
)

func TestEndpointEventHandler(t *testing.T) {

	type operation string
	const (
		add    operation = "add"
		update operation = "update"
		delete operation = "delete"
	)

	clusterID := "alpha"

	svc1 := NewQualifiedName("svc", "svc")

	unusedEP := NewQualifiedName("unused", "unused")
	ep1 := NewQualifiedName("service", "test")

	ip1 := "1.1.1.1"
	ip2 := "1.1.1.2"

	weight1 := 10

	logger := logrus.New()
	logger.SetOutput(ioutil.Discard)

	fakeCache := &MockCache{}
	fakeCache.
		On("UpdateBackends", svc1, clusterID, ep1, weight1, []string{ip1}).
		Return([]QualifiedName{svc1})
	fakeCache.
		On("UpdateBackends", svc1, clusterID, ep1, weight1, []string{ip1, ip2}).
		Return([]QualifiedName{svc1})
	fakeCache.
		On("RemoveBackends", svc1, clusterID, ep1).
		Return([]QualifiedName{svc1})

	fakeServiceNotifier := &MockServiceNotifier{}
	fakeServiceNotifier.On("NotifyServicesUpdated", []QualifiedName{svc1})

	fakeCanary := &MockCanary{}
	fakeCanary.On("GetMappings", ep1, clusterID).Return(map[QualifiedName]EndpointMapping{
		svc1: NewEndpointMapping(ep1, svc1, weight1),
	})
	fakeCanary.On("GetMappings", unusedEP, clusterID).Return(map[QualifiedName]EndpointMapping{})

	epHandler := NewEndpointEventHandler(logger, fakeCache, fakeServiceNotifier, fakeCanary, clusterID)

	tests := []struct {
		name      string
		op        operation
		epHandler *EndpointEventHandler
		obj       interface{}
	}{
		{
			name:      "add ep",
			op:        add,
			epHandler: epHandler,
			obj: &v1.Endpoints{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: ep1.Namespace,
					Name:      ep1.Name,
				},
				Subsets: []v1.EndpointSubset{
					{
						Addresses: []v1.EndpointAddress{
							{IP: ip1},
						},
					},
				},
			},
		},
		{
			name:      "add unused ep",
			op:        update,
			epHandler: epHandler,
			obj: &v1.Endpoints{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: unusedEP.Namespace,
					Name:      unusedEP.Name,
				},
				Subsets: []v1.EndpointSubset{
					{
						Addresses: []v1.EndpointAddress{
							{IP: ip1},
						},
					},
				},
			},
		},
		{
			name:      "update ep",
			op:        add,
			epHandler: epHandler,
			obj: &v1.Endpoints{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: ep1.Namespace,
					Name:      ep1.Name,
				},
				Subsets: []v1.EndpointSubset{
					{
						Addresses: []v1.EndpointAddress{
							{IP: ip1},
							{IP: ip2},
						},
					},
				},
			},
		},
		{
			name:      "delete ep",
			op:        delete,
			epHandler: epHandler,
			obj: &v1.Endpoints{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: ep1.Namespace,
					Name:      ep1.Name,
				},
				Subsets: []v1.EndpointSubset{
					{
						Addresses: []v1.EndpointAddress{
							{IP: ip1},
							{IP: ip2},
						},
					},
				},
			},
		},
		{
			name:      "delete unknown ep",
			op:        delete,
			epHandler: epHandler,
			obj: k8sCache.DeletedFinalStateUnknown{
				Obj: &v1.Endpoints{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: ep1.Namespace,
						Name:      ep1.Name,
					},
					Subsets: []v1.EndpointSubset{
						{
							Addresses: []v1.EndpointAddress{
								{IP: ip1},
								{IP: ip2},
							},
						},
					},
				},
			},
		},
		{
			name:      "add wrong type",
			op:        delete,
			epHandler: epHandler,
			obj:       struct{}{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			switch tt.op {
			case add:
				assert.NotPanics(t, func() { tt.epHandler.OnAdd(tt.obj) })
			case update:
				assert.NotPanics(t, func() { tt.epHandler.OnUpdate(nil, tt.obj) })
			case delete:
				assert.NotPanics(t, func() { tt.epHandler.OnDelete(tt.obj) })
			}
		})
	}

	fakeServiceNotifier.AssertNotCalled(t, "NotifyServicesUpdated", nil)
	fakeServiceNotifier.AssertExpectations(t)
	fakeCanary.AssertExpectations(t)
	fakeCache.AssertExpectations(t)
}

func TestServiceEventHandler(t *testing.T) {

	type operation string
	const (
		add    operation = "add"
		update operation = "update"
		delete operation = "delete"
	)

	clusterID := "alpha"

	ip1 := "1.1.1.1"
	ip2 := "1.1.1.2"

	port := 8080

	svc1 := NewQualifiedName("services", "service")
	kubePorts := []v1.ServicePort{
		{
			Name:       "http",
			Protocol:   "http",
			Port:       int32(port),
			TargetPort: intstr.FromInt(port),
		},
	}
	k8sPorts := []Port{
		{
			Name:       "http",
			Protocol:   "http",
			Port:       port,
			TargetPort: port,
		},
	}

	logger := logrus.New()
	logger.SetOutput(ioutil.Discard)

	fakeCache := &MockCache{}
	fakeCache.
		On("UpdateService", svc1.Namespace, svc1.Name, clusterID, ip1, k8sPorts).
		Return([]QualifiedName{svc1})
	fakeCache.
		On("UpdateService", svc1.Namespace, svc1.Name, clusterID, ip2, k8sPorts).
		Return([]QualifiedName(nil))
	fakeCache.
		On("RemoveService", svc1.Namespace, svc1.Name, clusterID).
		Return([]QualifiedName{svc1})

	// check cache
	fakeCache.On("GetSnapshot").Return(map[string]map[string]*Service{})
	fakeCache.GetSnapshot()

	fakeIngressCache := &MockIngressCache{}
	fakeIngressCache.
		On("UpdateServicePort", clusterID, svc1.Namespace, svc1.Name, k8sPorts).Return([]*Ingress{})
	fakeServiceNotifier := &MockServiceNotifier{}
	fakeServiceNotifier.On("NotifyServicesUpdated", []QualifiedName{svc1})

	fakeIngressNotifier := &MockIngressNotifier{}

	svcHandler := NewServiceEventHandler(
		logger,
		fakeCache,
		fakeIngressCache,
		fakeServiceNotifier,
		fakeIngressNotifier,
		clusterID)

	service1 := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: svc1.Namespace,
			Name:      svc1.Name,
		},
		Spec: v1.ServiceSpec{
			ClusterIP: ip1,
			Ports:     kubePorts,
		},
	}

	service2 := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: svc1.Namespace,
			Name:      svc1.Name,
		},
		Spec: v1.ServiceSpec{
			ClusterIP: ip2,
			Ports:     kubePorts,
		},
	}

	externalService := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: svc1.Namespace,
			Name:      svc1.Name,
		},
		Spec: v1.ServiceSpec{
			Type:      v1.ServiceTypeExternalName,
			ClusterIP: ip2,
			Ports:     kubePorts,
		},
	}

	tests := []struct {
		name       string
		op         operation
		svcHandler *ServiceEventHandler
		obj        interface{}
	}{
		{
			name:       "add service",
			op:         add,
			svcHandler: svcHandler,
			obj:        service1,
		},
		{
			name:       "update same service",
			op:         update,
			svcHandler: svcHandler,
			obj:        service2,
		},
		{
			name:       "add same service",
			op:         add,
			svcHandler: svcHandler,
			obj:        service2,
		},
		{
			name:       "delete service",
			op:         delete,
			svcHandler: svcHandler,
			obj:        service2,
		},
		{
			name:       "delete same service",
			op:         delete,
			svcHandler: svcHandler,
			obj: k8sCache.DeletedFinalStateUnknown{
				Obj: service2,
			},
		},
		{
			name:       "delete external service",
			op:         delete,
			svcHandler: svcHandler,
			obj:        externalService,
		},
		{
			name:       "add wrong type",
			op:         delete,
			svcHandler: svcHandler,
			obj:        struct{}{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			switch tt.op {
			case add:
				assert.NotPanics(t, func() { tt.svcHandler.OnAdd(tt.obj) })
			case update:
				assert.NotPanics(t, func() { tt.svcHandler.OnUpdate(nil, tt.obj) })
			case delete:
				assert.NotPanics(t, func() { tt.svcHandler.OnDelete(tt.obj) })
			}
		})
	}

	fakeServiceNotifier.AssertNotCalled(t, "NotifyServicesUpdated", nil)
	fakeServiceNotifier.AssertExpectations(t)
	fakeCache.AssertExpectations(t)
}

func TestCanaryReleaseEventHandler(t *testing.T) {

	type operation string
	const (
		add    operation = "add"
		update operation = "update"
		delete operation = "delete"
	)

	svc1 := NewQualifiedName("services", "service")
	ep1 := NewQualifiedName("service", "test")
	clusterID := "alpha"
	weight1 := 10
	ip1 := "1.1.1.1"

	logger := logrus.New()
	logger.SetOutput(ioutil.Discard)

	fakeCache := &MockCache{}
	fakeCache.On("FlushServiceByClusterID", svc1, clusterID)
	fakeCache.
		On("UpdateBackends", svc1, clusterID, svc1, EndpointsWeightSumForCluster, []string{ip1}).
		Return([]QualifiedName{svc1})

	fakeCanary := &MockCanary{}
	fakeCanary.
		On("UpdateMapping", svc1, clusterID, []EndpointMapping{NewEndpointMapping(svc1, svc1, weight1)}).
		Return()
	fakeCanary.
		On("UpdateMapping", svc1, clusterID, []EndpointMapping{
			NewEndpointMapping(svc1, svc1, weight1),
			NewEndpointMapping(ep1, svc1, weight1),
		})
	fakeCanary.On("DeleteMapping", svc1, clusterID)
	fakeCanary.On("GetDefaultMapping", svc1).Return(NewEndpointMapping(svc1, svc1, EndpointsWeightSumForCluster))
	fakeCanary.On("GetMappings", svc1, clusterID).Return(map[QualifiedName]EndpointMapping{
		svc1: NewEndpointMapping(svc1, svc1, EndpointsWeightSumForCluster),
	})

	fakeServiceNotifier := &MockServiceNotifier{}
	fakeServiceNotifier.On("NotifyServicesUpdated", []QualifiedName{svc1})

	// prepare endpoints lister with one EPs
	kubeEPs := &v1.Endpoints{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: svc1.Namespace,
			Name:      svc1.Name,
		},
		Subsets: []v1.EndpointSubset{
			{
				Addresses: []v1.EndpointAddress{
					{IP: ip1},
				},
			},
		},
	}
	cs := fakekube.NewSimpleClientset()
	fakeSharedInformerFactory := informers.NewSharedInformerFactory(cs, 0)
	fakeEndpointsInformer := fakeSharedInformerFactory.Core().V1().Endpoints()
	fakeEndpointsInformer.Informer().GetStore().Add(kubeEPs)
	fakeEndpointsLister := fakeEndpointsInformer.Lister()

	epHandler := NewEndpointEventHandler(logger, fakeCache, fakeServiceNotifier, fakeCanary, clusterID)
	canaryHandler := NewCanaryReleaseEventHandler(logger, fakeCache, fakeCanary, clusterID, fakeEndpointsLister, epHandler)

	canary1 := &navigatorV1.CanaryRelease{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: svc1.Namespace,
			Name:      svc1.Name,
		},
		Spec: navigatorV1.CanaryReleaseSpec{
			Backends: []navigatorV1.Backends{
				{
					Namespace: svc1.Namespace,
					Name:      svc1.Name,
					Weight:    weight1,
				},
			},
		},
	}

	canary2 := &navigatorV1.CanaryRelease{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: svc1.Namespace,
			Name:      svc1.Name,
		},
		Spec: navigatorV1.CanaryReleaseSpec{
			Backends: []navigatorV1.Backends{
				{
					Namespace: svc1.Namespace,
					Name:      svc1.Name,
					Weight:    weight1,
				},
				{
					Namespace: ep1.Namespace,
					Name:      ep1.Name,
					Weight:    weight1,
				},
			},
		},
	}

	tests := []struct {
		name          string
		op            operation
		canaryHandler *CanaryReleaseEventHandler
		obj           interface{}
	}{
		{
			name:          "add canary",
			op:            add,
			canaryHandler: canaryHandler,
			obj:           canary1,
		},
		{
			name:          "update same canary",
			op:            update,
			canaryHandler: canaryHandler,
			obj:           canary2,
		},
		{
			name:          "add same canary",
			op:            add,
			canaryHandler: canaryHandler,
			obj:           canary2,
		},
		{
			name:          "delete cancary",
			op:            delete,
			canaryHandler: canaryHandler,
			obj:           canary2,
		},
		{
			name:          "delete unknown canary",
			op:            add,
			canaryHandler: canaryHandler,
			obj: k8sCache.DeletedFinalStateUnknown{
				Obj: canary2,
			},
		},
		{
			name:          "add wrong type",
			op:            delete,
			canaryHandler: canaryHandler,
			obj:           struct{}{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			switch tt.op {
			case add:
				assert.NotPanics(t, func() { tt.canaryHandler.OnAdd(tt.obj) })
			case update:
				assert.NotPanics(t, func() { tt.canaryHandler.OnUpdate(nil, tt.obj) })
			case delete:
				assert.NotPanics(t, func() { tt.canaryHandler.OnDelete(tt.obj) })
			}
		})
	}

	fakeServiceNotifier.AssertNotCalled(t, "NotifyServicesUpdated", nil)
	fakeServiceNotifier.AssertExpectations(t)
	fakeCanary.AssertExpectations(t)
	fakeCache.AssertExpectations(t)
}

func TestNexusEventHandler(t *testing.T) {

	type operation string
	const (
		add    operation = "add"
		update operation = "update"
		delete operation = "delete"
	)

	clusterID := "alpha"

	app1 := "test-app"

	svc1 := NewQualifiedName("services", "service")
	svc2 := NewQualifiedName("test", "test-service")

	logger := logrus.New()
	logger.SetOutput(ioutil.Discard)

	fakeNexus := &MockNexusCache{}
	fakeNexus.On("Update", app1, svc1.Name, []QualifiedName{svc1}).Return([]string{app1})
	fakeNexus.On("Update", app1, svc1.Name, []QualifiedName{svc1, svc2}).Return([]string{app1})
	fakeNexus.On("Delete", app1, svc1.Name).Return([]string{app1})

	// check mock
	fakeNexus.On("GetSnapshot").Return(map[string]Nexus{}, map[QualifiedName][]string{})
	fakeNexus.GetSnapshot()

	fakeNexusNotifier := &MockNexusNotifier{}
	fakeNexusNotifier.On("NotifyNexusesUpdated", []string{app1}, false)

	nexusHandler := NewNexusEventHandler(
		logger,
		fakeNexus,
		fakeNexusNotifier,
		clusterID)

	nexus1 := &navigatorV1.Nexus{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: svc1.Namespace,
			Name:      svc1.Name,
		},
		Spec: navigatorV1.NexusSpec{
			AppName: app1,
			Services: []navigatorV1.Service{
				{
					Namespace: svc1.Namespace,
					Name:      svc1.Name,
				},
			},
		},
	}

	nexus2 := &navigatorV1.Nexus{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: svc1.Namespace,
			Name:      svc1.Name,
		},
		Spec: navigatorV1.NexusSpec{
			AppName: app1,
			Services: []navigatorV1.Service{
				{
					Namespace: svc1.Namespace,
					Name:      svc1.Name,
				},
				{
					Namespace: svc2.Namespace,
					Name:      svc2.Name,
				},
			},
		},
	}

	tests := []struct {
		name         string
		op           operation
		nexusHandler *NexusEventHandler
		obj          interface{}
	}{
		{
			name:         "add nexus",
			op:           add,
			nexusHandler: nexusHandler,
			obj:          nexus1,
		},
		{
			name:         "update same nexus",
			op:           update,
			nexusHandler: nexusHandler,
			obj:          nexus2,
		},
		{
			name:         "add same nexus",
			op:           add,
			nexusHandler: nexusHandler,
			obj:          nexus2,
		},
		{
			name:         "delete nexus",
			op:           delete,
			nexusHandler: nexusHandler,
			obj:          nexus2,
		},
		{
			name:         "delete same nexus",
			op:           delete,
			nexusHandler: nexusHandler,
			obj: k8sCache.DeletedFinalStateUnknown{
				Obj: nexus2,
			},
		},
		{
			name:         "add wrong type",
			op:           delete,
			nexusHandler: nexusHandler,
			obj:          struct{}{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			switch tt.op {
			case add:
				assert.NotPanics(t, func() { tt.nexusHandler.OnAdd(tt.obj) })
			case update:
				assert.NotPanics(t, func() { tt.nexusHandler.OnUpdate(nil, tt.obj) })
			case delete:
				assert.NotPanics(t, func() { tt.nexusHandler.OnDelete(tt.obj) })
			}
		})
	}
	time.Sleep(time.Second * 1)
	fakeNexusNotifier.AssertNotCalled(t, "NotifyNexusesUpdated", nil)
	fakeNexusNotifier.AssertExpectations(t)
	fakeNexus.AssertExpectations(t)
}

func TestIngressEventHandler(t *testing.T) {
	type operation string
	const (
		add    operation = "add"
		update operation = "update"
		delete operation = "delete"
	)

	clusterID := "alpha"

	app1 := "test-app"

	svc1 := NewQualifiedName("services", "service")
	svc2 := NewQualifiedName("test", "test-service")
	ingressQn := NewQualifiedName(app1, app1)
	ingressClass := ""
	services := []QualifiedName{NewQualifiedName(app1, svc1.Name)}

	ingress1 := &v1beta1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ingressQn.Name,
			Namespace: ingressQn.Namespace,
		},
		Spec: v1beta1.IngressSpec{
			Rules: []v1beta1.IngressRule{
				{
					Host: "domain.com",
					IngressRuleValue: v1beta1.IngressRuleValue{
						HTTP: &v1beta1.HTTPIngressRuleValue{
							Paths: []v1beta1.HTTPIngressPath{
								{
									Path: "/",
									Backend: v1beta1.IngressBackend{
										ServiceName: svc1.Name,
										ServicePort: intstr.FromInt(8890),
									},
								},
							},
						},
					},
				},
			},
		},
	}

	ingress2 := &v1beta1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ingressQn.Name,
			Namespace: ingressQn.Namespace,
		},
		Spec: v1beta1.IngressSpec{
			Rules: []v1beta1.IngressRule{
				{
					Host: "domain.com",
					IngressRuleValue: v1beta1.IngressRuleValue{
						HTTP: &v1beta1.HTTPIngressRuleValue{
							Paths: []v1beta1.HTTPIngressPath{
								{
									Path: "/service-another",
									Backend: v1beta1.IngressBackend{
										ServiceName: svc2.Name,
										ServicePort: intstr.FromInt(8890),
									},
								},
							},
						},
					},
				},
			},
		},
	}

	ing := &Ingress{Name: app1, Namespace: app1, ClusterID: clusterID, Class: ingressClass}

	logger := logrus.New()
	logger.SetOutput(ioutil.Discard)

	fakeIngress := &MockIngressCache{}
	ports := map[string]map[int]string{}
	fakeIngress.On("Update", clusterID, ingress1, ports).Return(ing)
	fakeIngress.On("Update", clusterID, ingress2, ports).Return(ing)
	fakeIngress.On("Remove", clusterID, ingress1).Return(ing)
	fakeIngress.On("Remove", clusterID, ingress2).Return(ing)
	fakeIngress.On("GetClusterIDsForIngress", ingressQn, ingressClass).Return([]string{clusterID})
	fakeIngress.On("GetClusterIDsForIngress", ingressQn, ingressClass).Return([]string{clusterID})
	fakeIngress.On("GetServicesByClass", clusterID, ingressClass).Return(services)

	fakeNexus := &MockNexusCache{}
	fakeNexus.On("Update", app1, app1, services).Return([]string{app1})

	fakeGateway := &MockGatewayCache{}
	fakeGateway.On("GatewaysByClass", clusterID, ingressClass).Return([]*Gateway{
		{
			Name:      app1,
			ClusterID: clusterID,
		},
	})

	fakek8sCache := &MockCache{}
	fakek8sCache.On("GetSnapshot").Return(map[string]map[string]*Service{
		svc1.Namespace: {
			svc1.Name: &Service{
				Namespace: svc1.Namespace,
				Name:      svc1.Name,
				Ports: []Port{
					{Port: 8890, Name: "http"},
				},
			},
		},
		svc2.Namespace: {
			svc2.Name: &Service{
				Namespace: svc2.Namespace,
				Name:      svc2.Name,
				Ports: []Port{
					{Port: 8890, Name: "http"},
				},
			},
		},
	})

	fakeNexusNotifier := &MockNexusNotifier{}
	fakeNexusNotifier.On("NotifyNexusesUpdated", []string{app1}, true)

	fakeIngressNotifier := &MockIngressNotifier{}
	fakeIngressNotifier.On("NotifyIngressUpdated", ing)
	fakeIngressNotifier.On("NotifyIngressUpdated", ing)

	ingressHandler := NewIngressEventHandler(
		logger,
		fakeNexus,
		fakeIngress,
		fakeGateway,
		fakek8sCache,
		fakeIngressNotifier,
		fakeNexusNotifier,
		clusterID,
	)

	tests := []struct {
		name           string
		op             operation
		ingressHandler *IngressEventHandler
		obj            interface{}
	}{
		{
			name:           "add ingress",
			op:             add,
			ingressHandler: ingressHandler,
			obj:            ingress1,
		},
		{
			name:           "update same ingress",
			op:             update,
			ingressHandler: ingressHandler,
			obj:            ingress2,
		},
		{
			name:           "add same ingress",
			op:             add,
			ingressHandler: ingressHandler,
			obj:            ingress2,
		},
		{
			name:           "delete ingress",
			op:             delete,
			ingressHandler: ingressHandler,
			obj:            ingress2,
		},
		{
			name:           "delete same ingress",
			op:             delete,
			ingressHandler: ingressHandler,
			obj: k8sCache.DeletedFinalStateUnknown{
				Obj: ingress2,
			},
		},
		{
			name:           "add wrong type",
			op:             delete,
			ingressHandler: ingressHandler,
			obj:            struct{}{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			switch tt.op {
			case add:
				assert.NotPanics(t, func() { tt.ingressHandler.OnAdd(tt.obj) })
			case update:
				assert.NotPanics(t, func() { tt.ingressHandler.OnUpdate(nil, tt.obj) })
			case delete:
				assert.NotPanics(t, func() { tt.ingressHandler.OnDelete(tt.obj) })
			}
		})
	}
	time.Sleep(time.Second * 1)
	fakeNexusNotifier.AssertNotCalled(t, "NotifyIngressUpdated", nil)
	fakeNexusNotifier.AssertExpectations(t)
	fakeNexus.AssertExpectations(t)
}

func TestGatewayEventHandler(t *testing.T) {
	type operation string
	const (
		add    operation = "add"
		update operation = "update"
		delete operation = "delete"
	)

	clusterID := "alpha"

	app1 := "test-app"
	ingressClass := ""
	port1 := 8890
	port2 := 8080
	services := []QualifiedName{}

	logger := logrus.New()
	logger.SetOutput(ioutil.Discard)

	gateway1 := &navigatorV1.Gateway{
		ObjectMeta: metav1.ObjectMeta{
			Name:      app1,
			Namespace: app1,
		},
		Spec: navigatorV1.GatewaySpec{
			IngressClass: ingressClass,
			Port:         port1,
		},
	}

	gateway2 := &navigatorV1.Gateway{
		ObjectMeta: metav1.ObjectMeta{
			Name:      app1,
			Namespace: app1,
		},
		Spec: navigatorV1.GatewaySpec{
			IngressClass: ingressClass,
			Port:         port2,
		},
	}

	gw := &Gateway{}

	fakeIngress := &MockIngressCache{}
	fakeIngress.On("GetServicesByClass", clusterID, ingressClass).Return(services)
	fakeNexus := &MockNexusCache{}
	fakeNexus.On("Update", app1, app1, services).Return([]string{app1})
	fakeNexus.On("Delete", app1, app1).Return([]string{app1})
	fakeGateway := &MockGatewayCache{}
	fakeGateway.On("Update", clusterID, gateway1).Return(gw)
	fakeGateway.On("Update", clusterID, gateway2).Return(gw)
	fakeGateway.On("Delete", clusterID, gateway1).Return(gw)
	fakeGateway.On("Delete", clusterID, gateway2).Return(gw)
	fakeNexusNotifier := &MockNexusNotifier{}
	fakeNexusNotifier.On("NotifyNexusesUpdated", []string{app1}, true)
	fakeGatewayNotifier := &MockGatewayNotifier{}
	fakeGatewayNotifier.On("NotifyGatewayUpdated", gw, false)
	fakeGatewayNotifier.On("NotifyGatewayUpdated", gw, true)

	gatewayHandler := NewGatewayEventHandler(
		logger,
		fakeIngress,
		fakeNexus,
		fakeGateway,
		fakeNexusNotifier,
		fakeGatewayNotifier,
		clusterID,
	)

	tests := []struct {
		name           string
		op             operation
		gatewayHandler *GatewayEventHandler
		obj            interface{}
	}{
		{
			name:           "add gateway",
			op:             add,
			gatewayHandler: gatewayHandler,
			obj:            gateway1,
		},
		{
			name:           "update same gateway",
			op:             update,
			gatewayHandler: gatewayHandler,
			obj:            gateway2,
		},
		{
			name:           "add same gateway",
			op:             add,
			gatewayHandler: gatewayHandler,
			obj:            gateway2,
		},
		{
			name:           "delete gateway",
			op:             delete,
			gatewayHandler: gatewayHandler,
			obj:            gateway2,
		},
		{
			name:           "delete same gateway",
			op:             delete,
			gatewayHandler: gatewayHandler,
			obj: k8sCache.DeletedFinalStateUnknown{
				Obj: gateway2,
			},
		},
		{
			name:           "add wrong type",
			op:             delete,
			gatewayHandler: gatewayHandler,
			obj:            struct{}{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			switch tt.op {
			case add:
				assert.NotPanics(t, func() { tt.gatewayHandler.OnAdd(tt.obj) })
			case update:
				assert.NotPanics(t, func() { tt.gatewayHandler.OnUpdate(nil, tt.obj) })
			case delete:
				assert.NotPanics(t, func() { tt.gatewayHandler.OnDelete(tt.obj) })
			}
		})
	}
	time.Sleep(time.Second * 1)
	fakeNexusNotifier.AssertNotCalled(t, "NotifyGatewayUpdated", nil)
	fakeNexusNotifier.AssertExpectations(t)
	fakeNexus.AssertExpectations(t)
}
