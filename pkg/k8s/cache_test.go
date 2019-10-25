// +build unit

package k8s

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
)

func TestCacheService(t *testing.T) {

	type args struct {
		namespace string
		name      string
		clusterID string
		ClusterIP string
		ports     []Port
	}

	type operation string
	const (
		update operation = "update"
		remove operation = "remove"
	)

	logger := logrus.New()
	logger.SetOutput(ioutil.Discard)
	c := NewCache(logger)

	ns1 := "test"
	ns2 := "service"

	service1 := "test-service"
	service2 := "service"

	cluster1 := "alpha"
	cluster2 := "beta"

	ip1 := "1.1.1.1"
	ip2 := "1.1.1.2"

	port1 := Port{
		Port:       80,
		TargetPort: 80,
		Protocol:   ProtocolTCP,
		Name:       "http",
	}
	port2 := Port{
		Port:       43,
		TargetPort: 43,
		Protocol:   ProtocolTCP,
		Name:       "tcp",
	}

	tests := []struct {
		name  string
		op    operation
		cache Cache
		args  args
		want  []QualifiedName
		// map[namespace]map[serviceName]*Service for GetSnapshot comparison
		// *Service equal any other *Service in this test
		wantServices map[string]map[string]*Service
	}{
		{
			name:  "update empty cache with new service",
			op:    update,
			cache: c,
			args: args{
				namespace: ns1,
				name:      service1,
				clusterID: cluster1,
				ClusterIP: ip1,
				ports:     []Port{port1, port2},
			},
			want: []QualifiedName{NewQualifiedName(ns1, service1)},
			wantServices: map[string]map[string]*Service{
				ns1: {service1: nil},
			},
		},
		{
			name:  "update with same service",
			op:    update,
			cache: c,
			args: args{
				namespace: ns1,
				name:      service1,
				clusterID: cluster1,
				ClusterIP: ip1,
				ports:     []Port{port1, port2},
			},
			want: nil,
			wantServices: map[string]map[string]*Service{
				ns1: {service1: nil},
			},
		},
		{
			name:  "update with same service in another cluster",
			op:    update,
			cache: c,
			args: args{
				namespace: ns1,
				name:      service1,
				clusterID: cluster2,
				ClusterIP: ip2,
				ports:     []Port{port1, port2},
			},
			want: []QualifiedName{NewQualifiedName(ns1, service1)},
			wantServices: map[string]map[string]*Service{
				ns1: {service1: nil},
			},
		},
		{
			name:  "update with new service",
			op:    update,
			cache: c,
			args: args{
				namespace: ns2,
				name:      service2,
				clusterID: cluster2,
				ClusterIP: ip2,
				ports:     []Port{port1, port2},
			},
			want: []QualifiedName{NewQualifiedName(ns2, service2)},
			wantServices: map[string]map[string]*Service{
				ns1: {service1: nil},
				ns2: {service2: nil},
			},
		},
		{
			name:  "update service in new namespace",
			op:    update,
			cache: c,
			args: args{
				namespace: ns1,
				name:      service2,
				clusterID: cluster2,
				ClusterIP: ip2,
				ports:     []Port{port1, port2},
			},
			want: []QualifiedName{NewQualifiedName(ns1, service2)},
			wantServices: map[string]map[string]*Service{
				ns1: {
					service1: nil,
					service2: nil,
				},
				ns2: {service2: nil},
			},
		},
		{
			name:  "remove service from namespace",
			op:    remove,
			cache: c,
			args: args{
				namespace: ns2,
				name:      service2,
				clusterID: cluster2,
			},
			want: []QualifiedName{NewQualifiedName(ns2, service2)},
			wantServices: map[string]map[string]*Service{
				ns1: {
					service1: nil,
					service2: nil,
				},
			},
		},
		{
			name:  "remove service",
			op:    remove,
			cache: c,
			args: args{
				namespace: ns1,
				name:      service2,
				clusterID: cluster2,
			},
			want: []QualifiedName{NewQualifiedName(ns1, service2)},
			wantServices: map[string]map[string]*Service{
				ns1: {service1: nil},
			},
		},
		{
			name:  "remove removed service",
			op:    remove,
			cache: c,
			args: args{
				namespace: ns1,
				name:      service2,
				clusterID: cluster2,
			},
			want: nil,
			wantServices: map[string]map[string]*Service{
				ns1: {service1: nil},
			},
		},
		{
			name:  "remove service from cluster",
			op:    remove,
			cache: c,
			args: args{
				namespace: ns1,
				name:      service1,
				clusterID: cluster2,
			},
			want: []QualifiedName{NewQualifiedName(ns1, service1)},
			wantServices: map[string]map[string]*Service{
				ns1: {service1: nil},
			},
		},
		{
			name:  "remove all services",
			op:    remove,
			cache: c,
			args: args{
				namespace: ns1,
				name:      service1,
				clusterID: cluster1,
			},
			want:         []QualifiedName{NewQualifiedName(ns1, service1)},
			wantServices: map[string]map[string]*Service{},
		},
		{
			name:  "remove from empty cache",
			op:    remove,
			cache: c,
			args: args{
				namespace: ns1,
				name:      service1,
				clusterID: cluster1,
			},
			want:         nil,
			wantServices: map[string]map[string]*Service{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got []QualifiedName
			switch tt.op {
			case update:
				got = tt.cache.UpdateService(tt.args.namespace, tt.args.name, tt.args.clusterID, tt.args.ClusterIP, tt.args.ports)
			case remove:
				got = tt.cache.RemoveService(tt.args.namespace, tt.args.name, tt.args.clusterID)
			}
			assert.ElementsMatch(t, tt.want, got)

			snap := tt.cache.GetSnapshot()
			opt := cmp.Comparer(func(want, got *Service) bool {
				return true
			})
			cmpSnapFunc := func() bool {
				return cmp.Equal(tt.wantServices, snap, opt)
			}
			assert.Conditionf(t, cmpSnapFunc, "diff:\n%s", cmp.Diff(tt.wantServices, snap, opt))
		})
	}
}

func TestCacheBackend(t *testing.T) {

	type args struct {
		serviceName     QualifiedName
		clusterID       string
		endpointSetName QualifiedName
		weight          int
		ips             []string
	}

	type operation string
	const (
		update operation = "update"
		remove operation = "remove"
		flush  operation = "flush"
	)

	logger := logrus.New()
	logger.SetOutput(ioutil.Discard)
	c := NewCache(logger)

	cluster1 := "alpha"
	cluster2 := "beta"

	ns1 := "test"
	ns2 := "service"

	name1 := "test-service"
	name2 := "service"

	service1 := NewQualifiedName(ns1, name1)
	service2 := NewQualifiedName(ns2, name2)
	service3 := NewQualifiedName("unknown", "unknown")

	ep11 := NewQualifiedName(ns1, name1+"-v1")
	ep12 := NewQualifiedName(ns1, name1+"-v2")

	ep21 := NewQualifiedName(ns2, name2+"-v1")
	ep22 := NewQualifiedName(ns2, name2+"-v2")

	weight1 := 10
	weight2 := 100

	ips11 := []string{"1.1.1.1", "1.1.1.2", "1.1.1.3"}
	ips12 := []string{"1.1.2.1"}

	ips21 := []string{"1.2.1.1", "1.2.1.2", "1.2.1.3"}
	ips22 := []string{"1.2.2.1"}

	tests := []struct {
		name  string
		op    operation
		cache Cache
		args  args
		want  []QualifiedName
		// map[namespace]map[serviceName]*Service for GetSnapshot comparison
		// *Service equal any other *Service in this test
		wantServices map[string]map[string]*Service
	}{
		{
			name:  "update empty cache with new service",
			op:    update,
			cache: c,
			args: args{
				serviceName:     service1,
				clusterID:       cluster1,
				endpointSetName: ep11,
				weight:          weight1,
				ips:             ips11,
			},
			want: []QualifiedName{service1},
			wantServices: map[string]map[string]*Service{
				ns1: {name1: nil},
			},
		},
		{
			name:  "update with same ep",
			op:    update,
			cache: c,
			args: args{
				serviceName:     service1,
				clusterID:       cluster1,
				endpointSetName: ep11,
				weight:          weight1,
				ips:             ips11,
			},
			want: nil,
			wantServices: map[string]map[string]*Service{
				ns1: {name1: nil},
			},
		},
		{
			name:  "update with new endpoint ips",
			op:    update,
			cache: c,
			args: args{
				serviceName:     service1,
				clusterID:       cluster1,
				endpointSetName: ep11,
				weight:          weight1,
				ips:             ips12,
			},
			want: []QualifiedName{service1},
			wantServices: map[string]map[string]*Service{
				ns1: {name1: nil},
			},
		},
		{
			name:  "update service with new endpoint and weight",
			op:    update,
			cache: c,
			args: args{
				serviceName:     service1,
				clusterID:       cluster1,
				endpointSetName: ep12,
				weight:          weight2,
				ips:             ips22,
			},
			want: []QualifiedName{service1},
			wantServices: map[string]map[string]*Service{
				ns1: {name1: nil},
			},
		},
		{
			name:  "update service with new cluster",
			op:    update,
			cache: c,
			args: args{
				serviceName:     service1,
				clusterID:       cluster2,
				endpointSetName: ep11,
				weight:          weight1,
				ips:             ips11,
			},
			want: []QualifiedName{service1},
			wantServices: map[string]map[string]*Service{
				ns1: {name1: nil},
			},
		},
		{
			name:  "update with new service",
			op:    update,
			cache: c,
			args: args{
				serviceName:     service2,
				clusterID:       cluster2,
				endpointSetName: ep21,
				weight:          weight2,
				ips:             ips21,
			},
			want: []QualifiedName{service2},
			wantServices: map[string]map[string]*Service{
				ns1: {name1: nil},
				ns2: {name2: nil},
			},
		},
		{
			name:  "remove service backends",
			op:    remove,
			cache: c,
			args: args{
				serviceName:     service2,
				clusterID:       cluster2,
				endpointSetName: ep21,
			},
			want: []QualifiedName{service2},
			wantServices: map[string]map[string]*Service{
				ns1: {name1: nil},
				ns2: {name2: nil},
			},
		},
		{
			name:  "remove same service backends",
			op:    remove,
			cache: c,
			args: args{
				serviceName:     service1,
				clusterID:       cluster2,
				endpointSetName: ep21,
			},
			want: nil,
			wantServices: map[string]map[string]*Service{
				ns1: {name1: nil},
				ns2: {name2: nil},
			},
		},
		{
			name:  "remove backends of unknown service",
			op:    remove,
			cache: c,
			args: args{
				serviceName:     service3,
				clusterID:       cluster2,
				endpointSetName: ep21,
			},
			want: nil,
			wantServices: map[string]map[string]*Service{
				ns1: {name1: nil},
				ns2: {name2: nil},
			},
		},
		{
			name:  "flush service backends with no backends",
			op:    flush,
			cache: c,
			args: args{
				serviceName: ep22,
				clusterID:   cluster1,
			},
			wantServices: map[string]map[string]*Service{
				ns1: {name1: nil},
				ns2: {name2: nil},
			},
		},
		{
			name:  "flush service backends",
			op:    flush,
			cache: c,
			args: args{
				serviceName: service1,
				clusterID:   cluster1,
			},
			wantServices: map[string]map[string]*Service{
				ns1: {name1: nil},
				ns2: {name2: nil},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got []QualifiedName
			switch tt.op {
			case update:
				got = tt.cache.UpdateBackends(tt.args.serviceName, tt.args.clusterID, tt.args.endpointSetName, tt.args.weight, tt.args.ips)
			case remove:
				got = tt.cache.RemoveBackends(tt.args.serviceName, tt.args.clusterID, tt.args.endpointSetName)
			case flush:
				tt.cache.FlushServiceByClusterID(tt.args.serviceName, tt.args.clusterID)
			}
			assert.ElementsMatch(t, tt.want, got)

			snap := tt.cache.GetSnapshot()
			opt := cmp.Comparer(func(want, got *Service) bool {
				return true
			})
			cmpSnapFunc := func() bool {
				return cmp.Equal(tt.wantServices, snap, opt)
			}
			assert.Conditionf(t, cmpSnapFunc, "diff:\n%s", cmp.Diff(tt.wantServices, snap, opt))
		})
	}
}

func TestCacheMetrics(t *testing.T) {
	type updateService struct {
		service   string
		clusterID string
	}

	type updateBackend struct {
		service, clusterID, endpoint string
		ips                          []string
	}

	updateServices := func(c Cache, services []updateService) {
		for i, us := range services {
			c.UpdateService(us.service, us.service, us.clusterID, fmt.Sprintf("10.0.0.%d", i), nil)
		}
	}

	updateBackends := func(c Cache, backends []updateBackend) {
		for _, ub := range backends {
			c.UpdateBackends(
				NewQualifiedName(ub.service, ub.service),
				ub.clusterID,
				NewQualifiedName(ub.endpoint, ub.endpoint),
				100,
				ub.ips,
			)
		}
	}

	tests := []struct {
		updateServicesAfterBackends      bool
		updateServices                   []updateService
		updateBackends                   []updateBackend
		removeServices                   []updateService
		removeBackends                   []updateBackend
		backendsMetrics, servicesMetrics string
	}{
		{
			updateServicesAfterBackends: false,
			updateServices: []updateService{
				{service: "test-1", clusterID: "cluster-1"},
				{service: "test-1", clusterID: "cluster-2"},
				{service: "test-2", clusterID: "cluster-1"},
				{service: "test-2", clusterID: "cluster-2"},
			},
			updateBackends: []updateBackend{
				{service: "test-1", clusterID: "cluster-1", endpoint: "test-1", ips: []string{"1.0.0.1", "1.0.0.2"}},
				{service: "test-1", clusterID: "cluster-2", endpoint: "test-1", ips: []string{"1.1.0.1", "1.1.0.2"}},

				{service: "test-2", clusterID: "cluster-1", endpoint: "test-2", ips: []string{"1.0.1.1", "1.0.1.2"}},
				{service: "test-2", clusterID: "cluster-1", endpoint: "test-2-v2", ips: []string{"1.0.1.3", "1.0.1.4"}},
				{service: "test-2", clusterID: "cluster-2", endpoint: "test-2", ips: []string{"1.1.1.1", "1.1.1.2"}},
				{service: "test-2", clusterID: "cluster-2", endpoint: "test-2", ips: []string{"1.1.1.2", "1.1.1.3", "1.1.1.4"}},
			},
			servicesMetrics: `
			# HELP navigator_k8s_cache_services_count Count of services, stored in k8s cache
			# TYPE navigator_k8s_cache_services_count gauge
			navigator_k8s_cache_services_count 2
			`,
			backendsMetrics: `
			# HELP navigator_k8s_cache_backends_count Count of backends by service, stored in k8s cache
			# TYPE navigator_k8s_cache_backends_count gauge
			navigator_k8s_cache_backends_count{clusterID="cluster-1",service="test-1.test-1"} 2
			navigator_k8s_cache_backends_count{clusterID="cluster-2",service="test-1.test-1"} 2
			navigator_k8s_cache_backends_count{clusterID="cluster-1",service="test-2.test-2"} 4
			navigator_k8s_cache_backends_count{clusterID="cluster-2",service="test-2.test-2"} 3
			`,
		},
		{
			updateServicesAfterBackends: true,
			updateServices: []updateService{
				{service: "test-1", clusterID: "cluster-1"},
				{service: "test-1", clusterID: "cluster-2"},
				{service: "test-2", clusterID: "cluster-1"},
				{service: "test-2", clusterID: "cluster-2"},
			},
			updateBackends: []updateBackend{
				{service: "test-1", clusterID: "cluster-1", endpoint: "test-1", ips: []string{"1.0.0.1", "1.0.0.2"}},
				{service: "test-1", clusterID: "cluster-2", endpoint: "test-1", ips: []string{"1.1.0.1", "1.1.0.2"}},

				{service: "test-2", clusterID: "cluster-1", endpoint: "test-2", ips: []string{"1.0.1.1", "1.0.1.2"}},
				{service: "test-2", clusterID: "cluster-1", endpoint: "test-2-v2", ips: []string{"1.0.1.3", "1.0.1.4"}},
				{service: "test-2", clusterID: "cluster-2", endpoint: "test-2", ips: []string{"1.1.1.1", "1.1.1.2"}},
			},
			removeServices: []updateService{
				{service: "test-1", clusterID: "cluster-1"},
				{service: "test-1", clusterID: "cluster-2"},
			},
			removeBackends: []updateBackend{
				{service: "test-2", clusterID: "cluster-1", endpoint: "test-2", ips: []string{"1.0.1.1", "1.0.1.2"}},
			},
			servicesMetrics: `
			# HELP navigator_k8s_cache_services_count Count of services, stored in k8s cache
			# TYPE navigator_k8s_cache_services_count gauge
			navigator_k8s_cache_services_count 1
			`,
			backendsMetrics: `
			# HELP navigator_k8s_cache_backends_count Count of backends by service, stored in k8s cache
			# TYPE navigator_k8s_cache_backends_count gauge
			navigator_k8s_cache_backends_count{clusterID="cluster-1",service="test-2.test-2"} 2
			navigator_k8s_cache_backends_count{clusterID="cluster-2",service="test-2.test-2"} 2
			`,
		},
	}

	for n, tt := range tests {
		logger := logrus.New()
		logger.SetOutput(ioutil.Discard)
		c := NewCache(logger)
		prometheusBackendsCount.Reset()
		prometheusServiceCount.Set(0)

		if !tt.updateServicesAfterBackends {
			updateServices(c, tt.updateServices)
			updateBackends(c, tt.updateBackends)
		} else {
			updateBackends(c, tt.updateBackends)
			updateServices(c, tt.updateServices)
		}

		for _, us := range tt.removeServices {
			c.RemoveService(us.service, us.service, us.clusterID)
		}
		for _, ub := range tt.removeBackends {
			c.RemoveBackends(
				NewQualifiedName(ub.service, ub.service),
				ub.clusterID,
				NewQualifiedName(ub.endpoint, ub.endpoint),
			)
		}

		servicesResult := testutil.CollectAndCompare(
			prometheusServiceCount,
			bytes.NewBuffer([]byte(tt.servicesMetrics)),
			"navigator_k8s_cache_services_count",
		)
		if servicesResult != nil {
			t.Errorf("Case %d navigator_k8s_cache_services_count metrics failed: %s", n, servicesResult)
		}

		backendsResult := testutil.CollectAndCompare(
			prometheusBackendsCount,
			bytes.NewBuffer([]byte(tt.backendsMetrics)),
			"navigator_k8s_cache_backends_count",
		)
		if backendsResult != nil {
			t.Errorf("Case %d navigator_k8s_cache_backends_count metrics failed: %s", n, backendsResult)
		}
	}
}
