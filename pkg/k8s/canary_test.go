// +build unit

package k8s

import (
	"bytes"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/assert"
)

func TestCanaryMapping(t *testing.T) {

	type args struct {
		ServiceName QualifiedName
		clusterID   string
		mappings    []EndpointMapping
	}

	type wantMapping struct {
		backend BackendSourceID
		mapping map[QualifiedName]EndpointMapping
	}

	type operation string
	const (
		update operation = "update"
		delete operation = "delete"
	)

	ns1 := "test"
	ns2 := "service"
	ns3 := "foobar"

	serviceName1 := "test-service"
	serviceName2 := "service"

	service1 := NewQualifiedName(ns1, serviceName1)
	service2 := NewQualifiedName(ns2, serviceName2)

	service11 := NewQualifiedName(ns1, serviceName1+"-v1")
	service12 := NewQualifiedName(ns3, serviceName1+"-v2")

	service21 := NewQualifiedName(ns2, serviceName2+"-v1")
	service22 := NewQualifiedName(ns3, serviceName2+"-v2")

	weight1 := 10
	weight2 := 30

	cluster1 := "alpha"
	cluster2 := "beta"

	c := NewCanary()

	tests := []struct {
		name         string
		op           operation
		canary       Canary
		args         args
		wantMappings []wantMapping
	}{
		{
			name:   "try to delete from empty canary",
			op:     delete,
			canary: c,
			args: args{
				ServiceName: service1,
				clusterID:   cluster1,
			},
			wantMappings: []wantMapping{
				{
					backend: NewBackendSourceID(service1, cluster1),
					mapping: map[QualifiedName]EndpointMapping{service1: NewEndpointMapping(service1, service1, EndpointsWeightSumForCluster)},
				},
				{
					backend: NewBackendSourceID(service2, cluster2),
					mapping: map[QualifiedName]EndpointMapping{service2: NewEndpointMapping(service2, service2, EndpointsWeightSumForCluster)},
				},
			},
		},
		{
			name:   "update with empty canary",
			op:     update,
			canary: c,
			args: args{
				ServiceName: service1,
				clusterID:   cluster1,
				mappings: []EndpointMapping{
					NewEndpointMapping(service1, service1, weight1),
					NewEndpointMapping(service11, service1, weight2),
				},
			},
			wantMappings: []wantMapping{
				{
					backend: NewBackendSourceID(service1, cluster1),
					mapping: map[QualifiedName]EndpointMapping{
						service1: NewEndpointMapping(service1, service1, weight1),
					},
				},
				{
					backend: NewBackendSourceID(service11, cluster1),
					mapping: map[QualifiedName]EndpointMapping{
						service1:  NewEndpointMapping(service11, service1, weight2),
						service11: NewEndpointMapping(service11, service11, EndpointsWeightSumForCluster),
					},
				},
			},
		},
		{
			name:   "update same service in same cluster",
			op:     update,
			canary: c,
			args: args{
				ServiceName: service1,
				clusterID:   cluster1,
				mappings: []EndpointMapping{
					NewEndpointMapping(service11, service1, weight1),
					NewEndpointMapping(service12, service1, weight2),
				},
			},
			wantMappings: []wantMapping{
				{
					backend: NewBackendSourceID(service1, cluster1),
					mapping: map[QualifiedName]EndpointMapping{},
				},
				{
					backend: NewBackendSourceID(service11, cluster1),
					mapping: map[QualifiedName]EndpointMapping{
						service1:  NewEndpointMapping(service11, service1, weight1),
						service11: NewEndpointMapping(service11, service11, EndpointsWeightSumForCluster),
					},
				},
				{
					backend: NewBackendSourceID(service12, cluster1),
					mapping: map[QualifiedName]EndpointMapping{
						service1:  NewEndpointMapping(service12, service1, weight2),
						service12: NewEndpointMapping(service12, service12, EndpointsWeightSumForCluster),
					},
				},
			},
		},
		{
			name:   "update idempotent",
			op:     update,
			canary: c,
			args: args{
				ServiceName: service1,
				clusterID:   cluster1,
				mappings: []EndpointMapping{
					NewEndpointMapping(service11, service1, weight1),
					NewEndpointMapping(service12, service1, weight2),
				},
			},
			wantMappings: []wantMapping{
				{
					backend: NewBackendSourceID(service1, cluster1),
					mapping: map[QualifiedName]EndpointMapping{},
				},
				{
					backend: NewBackendSourceID(service1, cluster2),
					mapping: map[QualifiedName]EndpointMapping{
						service1: NewEndpointMapping(service1, service1, EndpointsWeightSumForCluster),
					},
				},
				{
					backend: NewBackendSourceID(service11, cluster1),
					mapping: map[QualifiedName]EndpointMapping{
						service1:  NewEndpointMapping(service11, service1, weight1),
						service11: NewEndpointMapping(service11, service11, EndpointsWeightSumForCluster),
					},
				},
				{
					backend: NewBackendSourceID(service12, cluster1),
					mapping: map[QualifiedName]EndpointMapping{
						service1:  NewEndpointMapping(service12, service1, weight2),
						service12: NewEndpointMapping(service12, service12, EndpointsWeightSumForCluster),
					},
				},
			},
		},
		{
			name:   "update service in another cluster",
			op:     update,
			canary: c,
			args: args{
				ServiceName: service1,
				clusterID:   cluster2,
				mappings: []EndpointMapping{
					NewEndpointMapping(service11, service1, weight1),
				},
			},
			wantMappings: []wantMapping{
				{
					backend: NewBackendSourceID(service1, cluster1),
					mapping: map[QualifiedName]EndpointMapping{},
				},
				{
					backend: NewBackendSourceID(service1, cluster2),
					mapping: map[QualifiedName]EndpointMapping{},
				},
				{
					backend: NewBackendSourceID(service11, cluster1),
					mapping: map[QualifiedName]EndpointMapping{
						service1:  NewEndpointMapping(service11, service1, weight1),
						service11: NewEndpointMapping(service11, service11, EndpointsWeightSumForCluster),
					},
				},
				{
					backend: NewBackendSourceID(service12, cluster1),
					mapping: map[QualifiedName]EndpointMapping{
						service1:  NewEndpointMapping(service12, service1, weight2),
						service12: NewEndpointMapping(service12, service12, EndpointsWeightSumForCluster),
					},
				},
				{
					backend: NewBackendSourceID(service11, cluster2),
					mapping: map[QualifiedName]EndpointMapping{
						service1:  NewEndpointMapping(service11, service1, weight1),
						service11: NewEndpointMapping(service11, service11, EndpointsWeightSumForCluster),
					},
				},
			},
		},
		{
			name:   "update with new service",
			op:     update,
			canary: c,
			args: args{
				ServiceName: service2,
				clusterID:   cluster1,
				mappings: []EndpointMapping{
					NewEndpointMapping(service2, service2, weight1),
					NewEndpointMapping(service21, service2, weight1),
				},
			},
			wantMappings: []wantMapping{
				{
					backend: NewBackendSourceID(service1, cluster1),
					mapping: map[QualifiedName]EndpointMapping{},
				},
				{
					backend: NewBackendSourceID(service1, cluster2),
					mapping: map[QualifiedName]EndpointMapping{},
				},
				{
					backend: NewBackendSourceID(service11, cluster1),
					mapping: map[QualifiedName]EndpointMapping{
						service1:  NewEndpointMapping(service11, service1, weight1),
						service11: NewEndpointMapping(service11, service11, EndpointsWeightSumForCluster),
					},
				},
				{
					backend: NewBackendSourceID(service12, cluster1),
					mapping: map[QualifiedName]EndpointMapping{
						service1:  NewEndpointMapping(service12, service1, weight2),
						service12: NewEndpointMapping(service12, service12, EndpointsWeightSumForCluster),
					},
				},
				{
					backend: NewBackendSourceID(service11, cluster2),
					mapping: map[QualifiedName]EndpointMapping{
						service1:  NewEndpointMapping(service11, service1, weight1),
						service11: NewEndpointMapping(service11, service11, EndpointsWeightSumForCluster),
					},
				},
				{
					backend: NewBackendSourceID(service2, cluster1),
					mapping: map[QualifiedName]EndpointMapping{
						service2: NewEndpointMapping(service2, service2, weight1),
					},
				},
				{
					backend: NewBackendSourceID(service21, cluster1),
					mapping: map[QualifiedName]EndpointMapping{
						service2:  NewEndpointMapping(service21, service2, weight1),
						service21: NewEndpointMapping(service21, service21, EndpointsWeightSumForCluster),
					},
				},
			},
		},
		{
			name:   "update one service with another service endpoint",
			op:     update,
			canary: c,
			args: args{
				ServiceName: service2,
				clusterID:   cluster2,
				mappings: []EndpointMapping{
					NewEndpointMapping(service22, service2, weight2),
					NewEndpointMapping(service11, service2, weight2),
				},
			},
			wantMappings: []wantMapping{
				{
					backend: NewBackendSourceID(service1, cluster1),
					mapping: map[QualifiedName]EndpointMapping{},
				},
				{
					backend: NewBackendSourceID(service1, cluster2),
					mapping: map[QualifiedName]EndpointMapping{},
				},
				{
					backend: NewBackendSourceID(service11, cluster1),
					mapping: map[QualifiedName]EndpointMapping{
						service1:  NewEndpointMapping(service11, service1, weight1),
						service11: NewEndpointMapping(service11, service11, EndpointsWeightSumForCluster),
					},
				},
				{
					backend: NewBackendSourceID(service12, cluster1),
					mapping: map[QualifiedName]EndpointMapping{
						service1:  NewEndpointMapping(service12, service1, weight2),
						service12: NewEndpointMapping(service12, service12, EndpointsWeightSumForCluster),
					},
				},
				{
					backend: NewBackendSourceID(service11, cluster2),
					mapping: map[QualifiedName]EndpointMapping{
						service1:  NewEndpointMapping(service11, service1, weight1),
						service11: NewEndpointMapping(service11, service11, EndpointsWeightSumForCluster),
						service2:  NewEndpointMapping(service11, service2, weight2),
					},
				},
				{
					backend: NewBackendSourceID(service2, cluster1),
					mapping: map[QualifiedName]EndpointMapping{
						service2: NewEndpointMapping(service2, service2, weight1),
					},
				},
				{
					backend: NewBackendSourceID(service2, cluster2),
					mapping: map[QualifiedName]EndpointMapping{},
				},
				{
					backend: NewBackendSourceID(service21, cluster1),
					mapping: map[QualifiedName]EndpointMapping{
						service2:  NewEndpointMapping(service21, service2, weight1),
						service21: NewEndpointMapping(service21, service21, EndpointsWeightSumForCluster),
					},
				},
				{
					backend: NewBackendSourceID(service22, cluster2),
					mapping: map[QualifiedName]EndpointMapping{
						service2:  NewEndpointMapping(service22, service2, weight2),
						service22: NewEndpointMapping(service22, service22, EndpointsWeightSumForCluster),
					},
				},
			},
		},
		{
			name:   "delete one service from one cluster",
			op:     delete,
			canary: c,
			args: args{
				ServiceName: service2,
				clusterID:   cluster1,
			},
			wantMappings: []wantMapping{
				{
					backend: NewBackendSourceID(service1, cluster1),
					mapping: map[QualifiedName]EndpointMapping{},
				},
				{
					backend: NewBackendSourceID(service1, cluster2),
					mapping: map[QualifiedName]EndpointMapping{},
				},
				{
					backend: NewBackendSourceID(service11, cluster1),
					mapping: map[QualifiedName]EndpointMapping{
						service1:  NewEndpointMapping(service11, service1, weight1),
						service11: NewEndpointMapping(service11, service11, EndpointsWeightSumForCluster),
					},
				},
				{
					backend: NewBackendSourceID(service12, cluster1),
					mapping: map[QualifiedName]EndpointMapping{
						service1:  NewEndpointMapping(service12, service1, weight2),
						service12: NewEndpointMapping(service12, service12, EndpointsWeightSumForCluster),
					},
				},
				{
					backend: NewBackendSourceID(service11, cluster2),
					mapping: map[QualifiedName]EndpointMapping{
						service1:  NewEndpointMapping(service11, service1, weight1),
						service11: NewEndpointMapping(service11, service11, EndpointsWeightSumForCluster),
						service2:  NewEndpointMapping(service11, service2, weight2),
					},
				},
				{
					backend: NewBackendSourceID(service2, cluster1),
					mapping: map[QualifiedName]EndpointMapping{
						service2: NewEndpointMapping(service2, service2, EndpointsWeightSumForCluster),
					},
				},
				{
					backend: NewBackendSourceID(service2, cluster2),
					mapping: map[QualifiedName]EndpointMapping{},
				},
				{
					backend: NewBackendSourceID(service21, cluster1),
					mapping: map[QualifiedName]EndpointMapping{
						service21: NewEndpointMapping(service21, service21, EndpointsWeightSumForCluster),
					},
				},
				{
					backend: NewBackendSourceID(service22, cluster2),
					mapping: map[QualifiedName]EndpointMapping{
						service2:  NewEndpointMapping(service22, service2, weight2),
						service22: NewEndpointMapping(service22, service22, EndpointsWeightSumForCluster),
					},
				},
			},
		},
		{
			name:   "delete all services from one cluster",
			op:     delete,
			canary: c,
			args: args{
				ServiceName: service1,
				clusterID:   cluster1,
			},
			wantMappings: []wantMapping{
				{
					backend: NewBackendSourceID(service1, cluster1),
					mapping: map[QualifiedName]EndpointMapping{
						service1: NewEndpointMapping(service1, service1, EndpointsWeightSumForCluster),
					},
				},
				{
					backend: NewBackendSourceID(service1, cluster2),
					mapping: map[QualifiedName]EndpointMapping{},
				},
				{
					backend: NewBackendSourceID(service11, cluster1),
					mapping: map[QualifiedName]EndpointMapping{
						service11: NewEndpointMapping(service11, service11, EndpointsWeightSumForCluster),
					},
				},
				{
					backend: NewBackendSourceID(service12, cluster1),
					mapping: map[QualifiedName]EndpointMapping{
						service12: NewEndpointMapping(service12, service12, EndpointsWeightSumForCluster),
					},
				},
				{
					backend: NewBackendSourceID(service11, cluster2),
					mapping: map[QualifiedName]EndpointMapping{
						service1:  NewEndpointMapping(service11, service1, weight1),
						service11: NewEndpointMapping(service11, service11, EndpointsWeightSumForCluster),
						service2:  NewEndpointMapping(service11, service2, weight2),
					},
				},
				{
					backend: NewBackendSourceID(service2, cluster2),
					mapping: map[QualifiedName]EndpointMapping{},
				},
				{
					backend: NewBackendSourceID(service22, cluster2),
					mapping: map[QualifiedName]EndpointMapping{
						service2:  NewEndpointMapping(service22, service2, weight2),
						service22: NewEndpointMapping(service22, service22, EndpointsWeightSumForCluster),
					},
				},
			},
		},
		{
			name:   "delete services with another service endpoint",
			op:     delete,
			canary: c,
			args: args{
				ServiceName: service1,
				clusterID:   cluster2,
			},
			wantMappings: []wantMapping{
				{
					backend: NewBackendSourceID(service1, cluster1),
					mapping: map[QualifiedName]EndpointMapping{
						service1: NewEndpointMapping(service1, service1, EndpointsWeightSumForCluster),
					},
				},
				{
					backend: NewBackendSourceID(service1, cluster2),
					mapping: map[QualifiedName]EndpointMapping{
						service1: NewEndpointMapping(service1, service1, EndpointsWeightSumForCluster),
					},
				},
				{
					backend: NewBackendSourceID(service11, cluster1),
					mapping: map[QualifiedName]EndpointMapping{
						service11: NewEndpointMapping(service11, service11, EndpointsWeightSumForCluster),
					},
				},
				{
					backend: NewBackendSourceID(service12, cluster1),
					mapping: map[QualifiedName]EndpointMapping{
						service12: NewEndpointMapping(service12, service12, EndpointsWeightSumForCluster),
					},
				},
				{
					backend: NewBackendSourceID(service11, cluster2),
					mapping: map[QualifiedName]EndpointMapping{
						service11: NewEndpointMapping(service11, service11, EndpointsWeightSumForCluster),
						service2:  NewEndpointMapping(service11, service2, weight2),
					},
				},
				{
					backend: NewBackendSourceID(service2, cluster2),
					mapping: map[QualifiedName]EndpointMapping{},
				},
				{
					backend: NewBackendSourceID(service22, cluster2),
					mapping: map[QualifiedName]EndpointMapping{
						service2:  NewEndpointMapping(service22, service2, weight2),
						service22: NewEndpointMapping(service22, service22, EndpointsWeightSumForCluster),
					},
				},
			},
		},
		{
			name:   "delete all",
			op:     delete,
			canary: c,
			args: args{
				ServiceName: service2,
				clusterID:   cluster2,
			},
			wantMappings: []wantMapping{
				{
					backend: NewBackendSourceID(service1, cluster1),
					mapping: map[QualifiedName]EndpointMapping{
						service1: NewEndpointMapping(service1, service1, EndpointsWeightSumForCluster),
					},
				},
				{
					backend: NewBackendSourceID(service1, cluster2),
					mapping: map[QualifiedName]EndpointMapping{
						service1: NewEndpointMapping(service1, service1, EndpointsWeightSumForCluster),
					},
				},
				{
					backend: NewBackendSourceID(service11, cluster1),
					mapping: map[QualifiedName]EndpointMapping{
						service11: NewEndpointMapping(service11, service11, EndpointsWeightSumForCluster),
					},
				},
				{
					backend: NewBackendSourceID(service12, cluster1),
					mapping: map[QualifiedName]EndpointMapping{
						service12: NewEndpointMapping(service12, service12, EndpointsWeightSumForCluster),
					},
				},
				{
					backend: NewBackendSourceID(service11, cluster2),
					mapping: map[QualifiedName]EndpointMapping{
						service11: NewEndpointMapping(service11, service11, EndpointsWeightSumForCluster),
					},
				},
				{
					backend: NewBackendSourceID(service2, cluster2),
					mapping: map[QualifiedName]EndpointMapping{
						service2: NewEndpointMapping(service2, service2, EndpointsWeightSumForCluster),
					},
				},
				{
					backend: NewBackendSourceID(service22, cluster2),
					mapping: map[QualifiedName]EndpointMapping{
						service22: NewEndpointMapping(service22, service22, EndpointsWeightSumForCluster),
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var testFunc func()
			switch tt.op {
			case update:
				testFunc = func() { tt.canary.UpdateMapping(tt.args.ServiceName, tt.args.clusterID, tt.args.mappings) }
			case delete:
				testFunc = func() { tt.canary.DeleteMapping(tt.args.ServiceName, tt.args.clusterID) }
			}
			assert.NotPanics(t, testFunc)

			for _, mapping := range tt.wantMappings {
				got := tt.canary.GetMappings(mapping.backend.EndpointSetName, mapping.backend.ClusterID)
				cmpFunc := func() bool {
					return cmp.Equal(mapping.mapping, got)
				}
				assert.Conditionf(t, cmpFunc, "diff:\n", cmp.Diff(mapping.mapping, got))
			}
		})
	}
}

func TestCanaryMetrics(t *testing.T) {

	type operation string
	const (
		update operation = "update"
		delete operation = "delete"
	)

	type args struct {
		op        operation
		service   QualifiedName
		clusterID string
		mappings  []EndpointMapping
	}

	cluster1 := "cluster-1"
	cluster2 := "cluster-beta"
	service1 := QualifiedName{
		Namespace: "test-1",
		Name:      "test-1",
	}
	service2 := QualifiedName{
		Namespace: "test-2",
		Name:      "test-22",
	}
	ep11 := service1
	ep12 := QualifiedName{
		Namespace: "magic",
		Name:      "magic-test",
	}
	ep21 := service1
	ep22 := service2

	tests := []struct {
		args            []args
		canariesMetrics string
	}{
		{
			args: []args{
				{
					op:        update,
					service:   service1,
					clusterID: cluster1,
					mappings: []EndpointMapping{
						{EndpointSetName: ep11, ServiceName: service1, Weight: 500},
						{EndpointSetName: ep12, ServiceName: service1, Weight: 500},
					},
				},
				{
					op:        update,
					service:   service1,
					clusterID: cluster2,
					mappings: []EndpointMapping{
						{EndpointSetName: ep12, ServiceName: service1, Weight: 1000},
					},
				},
				{
					op:        update,
					service:   service2,
					clusterID: cluster1,
					mappings: []EndpointMapping{
						{EndpointSetName: ep21, ServiceName: service1, Weight: 500},
					},
				},
				{
					op:        update,
					service:   service2,
					clusterID: cluster2,
					mappings: []EndpointMapping{
						{EndpointSetName: ep22, ServiceName: service1, Weight: 1000},
					},
				},
			},
			canariesMetrics: `
			# HELP navigator_canary_cache_canaries_count Count of canaried services
			# TYPE navigator_canary_cache_canaries_count gauge
			navigator_canary_cache_canaries_count 2
			`,
		},
		{
			args: []args{
				{
					op:        update,
					service:   service1,
					clusterID: cluster1,
					mappings: []EndpointMapping{
						{EndpointSetName: ep11, ServiceName: service1, Weight: 500},
						{EndpointSetName: ep12, ServiceName: service1, Weight: 500},
					},
				},
				{
					op:        update,
					service:   service1,
					clusterID: cluster2,
					mappings: []EndpointMapping{
						{EndpointSetName: ep12, ServiceName: service1, Weight: 1000},
					},
				},
				{
					op:        update,
					service:   service2,
					clusterID: cluster1,
					mappings: []EndpointMapping{
						{EndpointSetName: ep21, ServiceName: service1, Weight: 500},
					},
				},
				{
					op:        update,
					service:   service2,
					clusterID: cluster2,
					mappings: []EndpointMapping{
						{EndpointSetName: ep22, ServiceName: service1, Weight: 1000},
					},
				},
				{
					op:        delete,
					service:   service1,
					clusterID: cluster2,
				},
				{
					op:        delete,
					service:   service2,
					clusterID: cluster2,
				},
			},
			canariesMetrics: `
			# HELP navigator_canary_cache_canaries_count Count of canaried services
			# TYPE navigator_canary_cache_canaries_count gauge
			navigator_canary_cache_canaries_count 2
			`,
		},
		{
			args: []args{
				{
					op:        update,
					service:   service1,
					clusterID: cluster1,
					mappings: []EndpointMapping{
						{EndpointSetName: ep11, ServiceName: service1, Weight: 500},
						{EndpointSetName: ep12, ServiceName: service1, Weight: 500},
					},
				},
				{
					op:        update,
					service:   service1,
					clusterID: cluster2,
					mappings: []EndpointMapping{
						{EndpointSetName: ep12, ServiceName: service1, Weight: 1000},
					},
				},
				{
					op:        update,
					service:   service2,
					clusterID: cluster2,
					mappings: []EndpointMapping{
						{EndpointSetName: ep22, ServiceName: service1, Weight: 1000},
					},
				},
				{
					op:        delete,
					service:   service1,
					clusterID: cluster2,
				},
				{
					op:        delete,
					service:   service2,
					clusterID: cluster2,
				},
			},
			canariesMetrics: `
			# HELP navigator_canary_cache_canaries_count Count of canaried services
			# TYPE navigator_canary_cache_canaries_count gauge
			navigator_canary_cache_canaries_count 1
			`,
		},
		{
			args: []args{
				{
					op:        update,
					service:   service1,
					clusterID: cluster1,
					mappings: []EndpointMapping{
						{EndpointSetName: ep11, ServiceName: service1, Weight: 500},
						{EndpointSetName: ep12, ServiceName: service1, Weight: 500},
					},
				},
				{
					op:        update,
					service:   service1,
					clusterID: cluster2,
					mappings: []EndpointMapping{
						{EndpointSetName: ep12, ServiceName: service1, Weight: 1000},
					},
				},
				{
					op:        update,
					service:   service2,
					clusterID: cluster2,
					mappings: []EndpointMapping{
						{EndpointSetName: ep22, ServiceName: service1, Weight: 1000},
					},
				},
				{
					op:        delete,
					service:   service1,
					clusterID: cluster2,
				},
				{
					op:        delete,
					service:   service2,
					clusterID: cluster1,
				},
			},
			canariesMetrics: `
			# HELP navigator_canary_cache_canaries_count Count of canaried services
			# TYPE navigator_canary_cache_canaries_count gauge
			navigator_canary_cache_canaries_count 2
			`,
		},
		{
			args: []args{
				{
					op:        update,
					service:   service1,
					clusterID: cluster1,
					mappings: []EndpointMapping{
						{EndpointSetName: ep11, ServiceName: service1, Weight: 500},
						{EndpointSetName: ep12, ServiceName: service1, Weight: 500},
					},
				},
				{
					op:        update,
					service:   service2,
					clusterID: cluster2,
					mappings: []EndpointMapping{
						{EndpointSetName: ep22, ServiceName: service1, Weight: 1000},
					},
				},
				{
					op:        delete,
					service:   service1,
					clusterID: cluster1,
				},
				{
					op:        delete,
					service:   service2,
					clusterID: cluster2,
				},
			},
			canariesMetrics: `
			# HELP navigator_canary_cache_canaries_count Count of canaried services
			# TYPE navigator_canary_cache_canaries_count gauge
			navigator_canary_cache_canaries_count 0
			`,
		},
		{
			args: []args{
				{
					op:        update,
					service:   service1,
					clusterID: cluster1,
					mappings: []EndpointMapping{
						{EndpointSetName: ep11, ServiceName: service1, Weight: 500},
						{EndpointSetName: ep12, ServiceName: service1, Weight: 500},
					},
				},
				{
					op:        update,
					service:   service1,
					clusterID: cluster2,
					mappings: []EndpointMapping{
						{EndpointSetName: ep12, ServiceName: service1, Weight: 1000},
					},
				},
				{
					op:        delete,
					service:   service1,
					clusterID: cluster1,
				},
				{
					op:        delete,
					service:   service1,
					clusterID: cluster2,
				},
				{
					op:        update,
					service:   service2,
					clusterID: cluster2,
					mappings: []EndpointMapping{
						{EndpointSetName: ep22, ServiceName: service1, Weight: 1000},
					},
				},
				{
					op:        delete,
					service:   service2,
					clusterID: cluster2,
				},
			},
			canariesMetrics: `
			# HELP navigator_canary_cache_canaries_count Count of canaried services
			# TYPE navigator_canary_cache_canaries_count gauge
			navigator_canary_cache_canaries_count 0
			`,
		},
	}

	for n, tt := range tests {
		c := NewCanary()
		prometheusCanariesCount.Set(0)

		for _, script := range tt.args {
			switch script.op {
			case update:
				c.UpdateMapping(script.service, script.clusterID, script.mappings)
			case delete:
				c.DeleteMapping(script.service, script.clusterID)
			}
		}

		servicesResult := testutil.CollectAndCompare(
			prometheusCanariesCount,
			bytes.NewBuffer([]byte(tt.canariesMetrics)),
			"navigator_canary_cache_canaries_count",
		)
		if servicesResult != nil {
			t.Errorf("Case %d navigator_k8s_cache_services_count metrics failed: %s", n, servicesResult)
		}
	}
}
