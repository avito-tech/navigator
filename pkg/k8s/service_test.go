// +build unit

package k8s

import (
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/assert"
)

func TestQualifiedNameIsWholeNamespace(t *testing.T) {
	tests := []struct {
		name  string
		qname QualifiedName
		want  bool
	}{
		{
			name:  "whole namespace",
			qname: NewQualifiedName("app", ""),
			want:  true,
		},
		{
			name:  "not whole namespace",
			qname: NewQualifiedName("app", "ooh"),
			want:  false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.qname.IsWholeNamespace()
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestQualifiedNameWholeNamespaceKey(t *testing.T) {
	tests := []struct {
		name  string
		qname QualifiedName
		want  QualifiedName
	}{
		{
			name:  "whole namespace key from partial",
			qname: NewQualifiedName("services", "service"),
			want:  NewQualifiedName("services", ""),
		},
		{
			name:  "whole namespace key from whole namespace",
			qname: NewQualifiedName("services", ""),
			want:  NewQualifiedName("services", ""),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.qname.WholeNamespaceKey()
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestQualifiedNameString(t *testing.T) {
	tests := []struct {
		name    string
		qname   QualifiedName
		wantEnd string
	}{
		{
			name:    "whole namespace key from partial",
			qname:   NewQualifiedName("services", "service"),
			wantEnd: "service",
		},
		{
			name:    "whole namespace key from whole namespace",
			qname:   NewQualifiedName("services", ""),
			wantEnd: "*",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.qname.String()
			assert.Condition(t, func() bool { return strings.HasSuffix(got, tt.wantEnd) })
		})
	}
}

func TestAddressSetIsEqual(t *testing.T) {

	addr1 := "addr1"
	addr2 := "addr2"
	addr3 := "addr3"

	ip1 := "1.1.1.1"
	ip2 := "1.1.1.2"
	ip3 := "1.1.1.3"

	cluster1 := "alpha"
	cluster2 := "beta"
	cluster3 := "gamma"

	tests := []struct {
		name       string
		addressSet AddressSet
		arg        AddressSet
		want       bool
	}{
		{
			name:       "equal empty",
			addressSet: AddressSet{},
			arg:        AddressSet{},
			want:       true,
		},
		{
			name: "equal one",
			addressSet: AddressSet{
				addr1: Address{IP: ip1, ClusterID: cluster1},
			},
			arg: AddressSet{
				addr1: Address{IP: ip1, ClusterID: cluster1},
			},
			want: true,
		},
		{
			name: "not equal one by IP",
			addressSet: AddressSet{
				addr1: Address{IP: ip1, ClusterID: cluster1},
			},
			arg: AddressSet{
				addr1: Address{IP: ip2, ClusterID: cluster1},
			},
			want: false,
		},
		{
			name: "not equal one by cluster",
			addressSet: AddressSet{
				addr1: Address{IP: ip1, ClusterID: cluster1},
			},
			arg: AddressSet{
				addr1: Address{IP: ip1, ClusterID: cluster2},
			},
			want: false,
		},
		{
			name: "not equal one by name",
			addressSet: AddressSet{
				addr1: Address{IP: ip1, ClusterID: cluster1},
			},
			arg: AddressSet{
				addr2: Address{IP: ip1, ClusterID: cluster1},
			},
			want: false,
		},
		{
			name: "equal several",
			addressSet: AddressSet{
				addr1: Address{IP: ip1, ClusterID: cluster1},
				addr2: Address{IP: ip2, ClusterID: cluster2},
			},
			arg: AddressSet{
				addr1: Address{IP: ip1, ClusterID: cluster1},
				addr2: Address{IP: ip2, ClusterID: cluster2},
			},
			want: true,
		},
		{
			name: "not equal count",
			addressSet: AddressSet{
				addr1: Address{IP: ip1, ClusterID: cluster1},
			},
			arg: AddressSet{
				addr1: Address{IP: ip1, ClusterID: cluster1},
				addr2: Address{IP: ip2, ClusterID: cluster2},
			},
			want: false,
		},
		{
			name: "not equal empty",
			addressSet: AddressSet{
				addr1: Address{IP: ip1, ClusterID: cluster1},
				addr2: Address{IP: ip2, ClusterID: cluster2},
			},
			arg:  AddressSet{},
			want: false,
		},
		{
			name: "not equal several",
			addressSet: AddressSet{
				addr1: Address{IP: ip1, ClusterID: cluster1},
				addr2: Address{IP: ip2, ClusterID: cluster2},
			},
			arg: AddressSet{
				addr1: Address{IP: ip1, ClusterID: cluster1},
				addr3: Address{IP: ip3, ClusterID: cluster3},
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.addressSet.isEqual(tt.arg)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestAddressSetCopy(t *testing.T) {

	addr1 := "addr1"
	addr2 := "addr2"

	ip1 := "1.1.1.1"
	ip2 := "1.1.1.2"

	cluster1 := "alpha"
	cluster2 := "beta"

	tests := []struct {
		name       string
		addressSet AddressSet
	}{
		{
			name:       "copy empty",
			addressSet: AddressSet{},
		},
		{
			name: "copy one",
			addressSet: AddressSet{
				addr1: Address{IP: ip1, ClusterID: cluster1},
			},
		},
		{
			name: "copy several",
			addressSet: AddressSet{
				addr1: Address{IP: ip1, ClusterID: cluster1},
				addr2: Address{IP: ip2, ClusterID: cluster2},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.addressSet.Copy()

			// assert.(t, &tt.addressSet, &got)
			pointerCmp := func() bool {
				return &tt.addressSet != &got
			}
			assert.Conditionf(t, pointerCmp, "origin and copy data addresses are equal")

			dataCmp := func() bool {
				return cmp.Equal(tt.addressSet, got)
			}
			assert.Conditionf(t, dataCmp, "copy is not actually a copy, diff: \n%s", cmp.Diff(tt.addressSet, got))
		})
	}
}

func TestWeightedAddressSetIsEqual(t *testing.T) {

	addr1 := "addr1"
	addr2 := "addr2"

	ip1 := "1.1.1.1"
	ip2 := "1.1.1.2"

	cluster1 := "alpha"
	cluster2 := "beta"

	type waSet struct {
		weight int
		aSet   AddressSet
	}
	type args struct {
		other WeightedAddressSet
	}
	tests := []struct {
		name  string
		waSet waSet
		arg   waSet
		want  bool
	}{
		{
			name: "equal one",
			waSet: waSet{
				weight: 10,
				aSet: AddressSet{
					addr1: Address{IP: ip1, ClusterID: cluster1},
				},
			},
			arg: waSet{
				weight: 10,
				aSet: AddressSet{
					addr1: Address{IP: ip1, ClusterID: cluster1},
				},
			},
			want: true,
		},
		{
			name: "not equal one",
			waSet: waSet{
				weight: 10,
				aSet: AddressSet{
					addr1: Address{IP: ip1, ClusterID: cluster1},
				},
			},
			arg: waSet{
				weight: 1,
				aSet: AddressSet{
					addr1: Address{IP: ip1, ClusterID: cluster1},
				},
			},
			want: false,
		},
		{
			name: "equal several",
			waSet: waSet{
				weight: 10,
				aSet: AddressSet{
					addr1: Address{IP: ip1, ClusterID: cluster1},
					addr2: Address{IP: ip2, ClusterID: cluster2},
				},
			},
			arg: waSet{
				weight: 10,
				aSet: AddressSet{
					addr1: Address{IP: ip1, ClusterID: cluster1},
					addr2: Address{IP: ip2, ClusterID: cluster2},
				},
			},
			want: true,
		},
		{
			name: "not equal several",
			waSet: waSet{
				weight: 10,
				aSet: AddressSet{
					addr1: Address{IP: ip1, ClusterID: cluster1},
					addr2: Address{IP: ip2, ClusterID: cluster2},
				},
			},
			arg: waSet{
				weight: 10,
				aSet: AddressSet{
					addr1: Address{IP: ip1, ClusterID: cluster1},
					addr2: Address{IP: ip2, ClusterID: cluster2},
				},
			},
			want: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			wa := NewWeightedAddressSet(tt.waSet.weight)
			wa.AddrSet = tt.waSet.aSet

			waArg := NewWeightedAddressSet(tt.arg.weight)
			waArg.AddrSet = tt.arg.aSet

			got := wa.isEqual(waArg)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestWeightedAddressSetCopy(t *testing.T) {

	addr1 := "addr1"
	addr2 := "addr2"

	ip1 := "1.1.1.1"
	ip2 := "1.1.1.2"

	cluster1 := "alpha"
	cluster2 := "beta"

	weight := 10

	tests := []struct {
		name  string
		waSet WeightedAddressSet
		aSet  AddressSet
	}{
		{
			name:  "copy empty",
			waSet: NewWeightedAddressSet(weight),
			aSet:  AddressSet{},
		},
		{
			name:  "copy one",
			waSet: NewWeightedAddressSet(weight),
			aSet: AddressSet{
				addr1: Address{IP: ip1, ClusterID: cluster1},
			},
		},
		{
			name:  "copy several",
			waSet: NewWeightedAddressSet(weight),
			aSet: AddressSet{
				addr1: Address{IP: ip1, ClusterID: cluster1},
				addr2: Address{IP: ip2, ClusterID: cluster2},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.waSet.AddrSet = tt.aSet
			got := tt.waSet.Copy()

			pointerCmp := func() bool {
				return &tt.waSet != &got && &tt.waSet.AddrSet != &got.AddrSet
			}
			assert.Conditionf(t, pointerCmp, "origin and copy data addresses are equal")

			dataCmp := func() bool {
				return cmp.Equal(tt.waSet, got)
			}
			assert.Conditionf(t, dataCmp, "copy is not actually a copy, diff: \n%s", cmp.Diff(tt.waSet, got))
		})
	}
}

func TestServiceClusterIP(t *testing.T) {
	type args struct {
		clusterID string
		ip        string
	}
	type wantIP struct {
		ip string
		ok bool
	}

	type operation string
	const (
		update operation = "update"
		delete operation = "delete"
		get    operation = "get"
	)

	ns := "services"
	name := "test-service"

	cluster1 := "alpha"
	cluster2 := "beta"

	ip1 := "1.1.1.1"
	ip2 := "1.1.1.2"

	service := NewService(ns, name)

	tests := []struct {
		name    string
		op      operation
		service *Service
		args    args
		want    bool
		wantIP  string
	}{
		//
		// {
		// 	name: "update empty with empty ip string",
		// 	service: service,
		// 	args: args{
		// 		clusterID: cluster1,
		// 		ip: "",
		// 	},
		// 	want: true,
		// },
		{
			name:    "get cluster ip from empty service",
			op:      get,
			service: service,
			args: args{
				clusterID: cluster1,
			},
			want: false,
		},
		{
			name:    "update empty",
			op:      update,
			service: service,
			args: args{
				clusterID: cluster1,
				ip:        ip2,
			},
			want: true,
		},
		{
			name:    "get cluster ip",
			op:      get,
			service: service,
			args: args{
				clusterID: cluster1,
			},
			want:   true,
			wantIP: ip2,
		},
		{
			name:    "update",
			op:      update,
			service: service,
			args: args{
				clusterID: cluster1,
				ip:        ip1,
			},
			want: true,
		},
		{
			name:    "update with same ip",
			op:      update,
			service: service,
			args: args{
				clusterID: cluster1,
				ip:        ip1,
			},
			want: false,
		},
		{
			name:    "update with new cluster",
			op:      update,
			service: service,
			args: args{
				clusterID: cluster2,
				ip:        ip2,
			},
			want: true,
		},
		{
			// wtf test - metacluster
			name:    "update cluster with anohter cluster ip",
			op:      update,
			service: service,
			args: args{
				clusterID: cluster2,
				ip:        ip1,
			},
			want: true,
		},
		{
			name:    "delete cluster",
			op:      delete,
			service: service,
			args: args{
				clusterID: cluster2,
			},
			want: true,
		},
		{
			name:    "delete deleted cluster",
			op:      delete,
			service: service,
			args: args{
				clusterID: cluster2,
			},
			want: false,
		},
		{
			name:    "delete all clusters",
			op:      delete,
			service: service,
			args: args{
				clusterID: cluster1,
			},
			want: true,
		},
		{
			name:    "try to get cluster after delete",
			op:      get,
			service: service,
			args: args{
				clusterID: cluster1,
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got bool
			var gotIP string
			switch tt.op {
			case update:
				got = tt.service.UpdateClusterIP(tt.args.clusterID, tt.args.ip)
			case delete:
				got = tt.service.DeleteClusterIP(tt.args.clusterID)
			case get:
				gotIP, got = tt.service.GetClusterIP(tt.args.clusterID)

			}

			assert.Equal(t, tt.want, got)
			if tt.op == get && got == true {
				assert.Equal(t, tt.wantIP, gotIP)
			}

		})
	}
}

func TestServiceBackends(t *testing.T) {
	type args struct {
		clusterID       string
		endpointSetName QualifiedName
		weight          int
		ips             []string
	}

	type operation string
	const (
		update operation = "update"
		delete operation = "delete"
	)

	makeWASet := func(weight int, clusterID string, ips []string) WeightedAddressSet {
		waSet := NewWeightedAddressSet(weight)
		for _, ip := range ips {
			waSet.AddrSet[ip] = Address{
				ClusterID: clusterID,
				IP:        ip,
			}
		}
		return waSet
	}

	ns := "services"
	name := "test-service"

	cluster1 := "alpha"
	cluster2 := "beta"

	serviceEP := NewQualifiedName(ns, name)
	serviceWeight := 20
	serviceIPs := []string{"5.5.5.5"}

	ep1 := NewQualifiedName(ns, name+"-v1")
	ep2 := NewQualifiedName(ns, name+"-v2")

	weight1 := 10
	weight2 := 100

	ip1 := []string{"1.1.1.1", "1.1.1.2", "1.1.1.3"}
	ip2 := []string{"1.1.2.1"}

	service := NewService(ns, name)

	tests := []struct {
		name         string
		op           operation
		service      *Service
		args         args
		want         bool
		wantBackends map[BackendSourceID]WeightedAddressSet
	}{
		{
			name:    "delete from empty service",
			op:      delete,
			service: service,
			args: args{
				clusterID:       cluster1,
				endpointSetName: ep1,
			},
			want:         false,
			wantBackends: map[BackendSourceID]WeightedAddressSet{},
		},
		{
			name:    "update empty service",
			op:      update,
			service: service,
			args: args{
				clusterID:       cluster1,
				endpointSetName: serviceEP,
				weight:          serviceWeight,
				ips:             serviceIPs,
			},
			want: true,
			wantBackends: map[BackendSourceID]WeightedAddressSet{
				NewBackendSourceID(serviceEP, cluster1): makeWASet(serviceWeight, cluster1, serviceIPs),
			},
		},
		{
			name:    "update with new EP",
			op:      update,
			service: service,
			args: args{
				clusterID:       cluster1,
				endpointSetName: ep1,
				weight:          weight1,
				ips:             ip1,
			},
			want: true,
			wantBackends: map[BackendSourceID]WeightedAddressSet{
				NewBackendSourceID(serviceEP, cluster1): makeWASet(serviceWeight, cluster1, serviceIPs),
				NewBackendSourceID(ep1, cluster1):       makeWASet(weight1, cluster1, ip1),
			},
		},
		{
			name:    "update with same EP",
			op:      update,
			service: service,
			args: args{
				clusterID:       cluster1,
				endpointSetName: ep1,
				weight:          weight1,
				ips:             ip1,
			},
			want: false,
			wantBackends: map[BackendSourceID]WeightedAddressSet{
				NewBackendSourceID(serviceEP, cluster1): makeWASet(serviceWeight, cluster1, serviceIPs),
				NewBackendSourceID(ep1, cluster1):       makeWASet(weight1, cluster1, ip1),
			},
		},
		{
			name:    "update with new IPs",
			op:      update,
			service: service,
			args: args{
				clusterID:       cluster1,
				endpointSetName: ep1,
				weight:          weight1,
				ips:             ip2,
			},
			want: true,
			wantBackends: map[BackendSourceID]WeightedAddressSet{
				NewBackendSourceID(serviceEP, cluster1): makeWASet(serviceWeight, cluster1, serviceIPs),
				NewBackendSourceID(ep1, cluster1):       makeWASet(weight1, cluster1, ip2),
			},
		},
		{
			name:    "update with new weight",
			op:      update,
			service: service,
			args: args{
				clusterID:       cluster1,
				endpointSetName: ep1,
				weight:          weight2,
				ips:             ip2,
			},
			want: true,
			wantBackends: map[BackendSourceID]WeightedAddressSet{
				NewBackendSourceID(serviceEP, cluster1): makeWASet(serviceWeight, cluster1, serviceIPs),
				NewBackendSourceID(ep1, cluster1):       makeWASet(weight2, cluster1, ip2),
			},
		},
		{
			name:    "update with new cluster",
			op:      update,
			service: service,
			args: args{
				clusterID:       cluster2,
				endpointSetName: ep2,
				weight:          weight1,
				ips:             ip1,
			},
			want: true,
			wantBackends: map[BackendSourceID]WeightedAddressSet{
				NewBackendSourceID(serviceEP, cluster1): makeWASet(serviceWeight, cluster1, serviceIPs),
				NewBackendSourceID(ep1, cluster1):       makeWASet(weight2, cluster1, ip2),
				NewBackendSourceID(ep2, cluster2):       makeWASet(weight1, cluster2, ip1),
			},
		},
		{
			name:    "delete all cluster",
			op:      delete,
			service: service,
			args: args{
				clusterID:       cluster2,
				endpointSetName: ep2,
			},
			want: true,
			wantBackends: map[BackendSourceID]WeightedAddressSet{
				NewBackendSourceID(serviceEP, cluster1): makeWASet(serviceWeight, cluster1, serviceIPs),
				NewBackendSourceID(ep1, cluster1):       makeWASet(weight2, cluster1, ip2),
			},
		},
		{
			name:    "delete service EP",
			op:      delete,
			service: service,
			args: args{
				clusterID:       cluster1,
				endpointSetName: serviceEP,
			},
			want: true,
			wantBackends: map[BackendSourceID]WeightedAddressSet{
				NewBackendSourceID(ep1, cluster1): makeWASet(weight2, cluster1, ip2),
			},
		},
		{
			name:    "delete all",
			op:      delete,
			service: service,
			args: args{
				clusterID:       cluster1,
				endpointSetName: ep1,
			},
			want:         true,
			wantBackends: map[BackendSourceID]WeightedAddressSet{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got bool

			switch tt.op {
			case update:
				got = tt.service.UpdateBackends(tt.args.clusterID, tt.args.endpointSetName, tt.args.weight, tt.args.ips)
			case delete:
				got = tt.service.DeleteBackends(tt.args.clusterID, tt.args.endpointSetName)
			}
			assert.Equal(t, tt.want, got)

			backendsCmp := func() bool {
				return cmp.Equal(tt.wantBackends, tt.service.BackendsBySourceID)
			}
			assert.Conditionf(t, backendsCmp, "check internal state error, diff: \n%s", cmp.Diff(tt.wantBackends, tt.service.BackendsBySourceID))
		})
	}
}

func TestServiceUpdatePorts(t *testing.T) {
	type args struct {
		ports []Port
	}

	ns := "services"
	name := "test-service"

	service := NewService(ns, name)

	tests := []struct {
		name    string
		service *Service
		ports   []Port
		want    bool
	}{
		{
			name:    "update with nil",
			service: service,
			ports:   nil,
			want:    false,
		},
		{
			name:    "update port",
			service: service,
			ports: []Port{
				{
					Port:       80,
					TargetPort: 80,
					Protocol:   ProtocolTCP,
					Name:       "http",
				},
			},
			want: true,
		},
		{
			name:    "update same port",
			service: service,
			ports: []Port{
				{
					Port:       80,
					TargetPort: 80,
					Protocol:   ProtocolTCP,
					Name:       "http",
				},
			},
			want: false,
		},
		{
			name:    "add new port",
			service: service,
			ports: []Port{
				{
					Port:       80,
					TargetPort: 80,
					Protocol:   ProtocolTCP,
					Name:       "http",
				},
				{
					Port:       43,
					TargetPort: 43,
					Protocol:   ProtocolTCP,
					Name:       "tcp",
				},
			},
			want: true,
		},
		{
			name:    "update ports",
			service: service,
			ports: []Port{
				{
					Port:       80,
					TargetPort: 80,
					Protocol:   ProtocolTCP,
					Name:       "http",
				},
				{
					Port:       42,
					TargetPort: 42,
					Protocol:   ProtocolTCP,
					Name:       "tcp",
				},
			},
			want: true,
		},
		{
			name:    "delete all",
			service: service,
			ports:   nil,
			want:    true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.service.UpdatePorts(tt.ports)
			if got != tt.want {
				t.Errorf("Service.UpdatePorts() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestServiceCopy(t *testing.T) {

	ns := "services"
	name := "test-service"

	cluster1 := "alpha"
	cluster2 := "beta"

	ip1 := "1.1.1.1"
	ip2 := "1.1.1.2"

	ep1 := NewQualifiedName(ns, name+"-v1")
	ep2 := NewQualifiedName(ns, name+"-v2")

	weight1 := 10
	weight2 := 100

	ips1 := []string{"1.1.1.1", "1.1.1.2", "1.1.1.3"}
	ips2 := []string{"1.1.2.1"}

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

	service := NewService(ns, name)

	serviceAll := NewService(ns, name)
	serviceAll.UpdateClusterIP(cluster1, ip1)
	serviceAll.UpdateClusterIP(cluster2, ip2)
	serviceAll.UpdateBackends(cluster1, ep1, weight1, ips1)
	serviceAll.UpdateBackends(cluster2, ep2, weight2, ips2)
	serviceAll.UpdatePorts([]Port{port1, port2})

	tests := []struct {
		name    string
		service *Service
	}{
		{
			name:    "copy empty",
			service: service,
		},
		{
			name:    "copy with clusters and backends and ports",
			service: serviceAll,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.service.Copy()

			pointersCmp := func() bool {
				return &tt.service.ClusterIPs != &got.ClusterIPs || &tt.service.BackendsBySourceID != &got.BackendsBySourceID || &tt.service.Ports != &got.Ports
			}
			assert.Conditionf(t, pointersCmp, "origin and copy data addresses are equal")

			dataCmp := func() bool {
				return cmp.Equal(tt.service, got)
			}
			assert.Conditionf(t, dataCmp, "copy is not actually a copy, diff: \n%s", cmp.Diff(tt.service, got))
		})
	}
}

func TestServiceKey(t *testing.T) {

	ns := "services"
	name := "test-service"
	service := NewService(ns, name)

	tests := []struct {
		name    string
		service *Service
		want    QualifiedName
	}{
		{
			name:    "simple",
			service: service,
			want:    NewQualifiedName(ns, name),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.service.Key()
			assert.Equal(t, tt.want, got)
		})
	}
}
