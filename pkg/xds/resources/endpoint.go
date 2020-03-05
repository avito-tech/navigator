package resources

import (
	"sync"

	"github.com/avito-tech/navigator/pkg/k8s"
	api "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	endpoint "github.com/envoyproxy/go-control-plane/envoy/api/v2/endpoint"
	"github.com/envoyproxy/go-control-plane/pkg/cache"
	"github.com/golang/protobuf/ptypes/wrappers"
)

type EndpointCache struct {
	muUpdate sync.RWMutex

	*BranchedResourceCache

	localityEnabled bool
}

func NewEndpointCache(opts ...FuncOpt) *EndpointCache {
	e := &EndpointCache{
		BranchedResourceCache: NewBranchedResourceCache(cache.EndpointType),
	}
	SetOpts(e, opts...)
	return e
}

func (c *EndpointCache) SetLocalityEnabled(enabled bool) {
	c.localityEnabled = enabled
}

func (c *EndpointCache) UpdateServices(updated, deleted []*k8s.Service) {

	c.muUpdate.Lock()
	defer c.muUpdate.Unlock()

	updatedResource := map[string][]NamedProtoMessage{}
	deletedResources := map[string][]NamedProtoMessage{}

	for _, u := range updated {
		c.getServiceEndpoints(updatedResource, u)
	}
	for _, d := range deleted {
		c.getServiceEndpoints(deletedResources, d)
	}

	c.UpdateBranchedResources(updatedResource, deletedResources)
}

func (c *EndpointCache) UpdateIngresses(updated map[k8s.QualifiedName]*k8s.Ingress) {
	// do nothing
}

func (c *EndpointCache) UpdateGateway(gw *k8s.Gateway) {
	// do nothing
}

func (c *EndpointCache) getServiceEndpoints(storage map[string][]NamedProtoMessage, service *k8s.Service) {
	for _, p := range service.Ports {
		if p.Protocol != k8s.ProtocolTCP {
			continue
		}

		name := ClusterName(service.Namespace, service.Name, p.Name)
		clusterIDs := map[string]struct{}{}
		for sid := range service.BackendsBySourceID {
			clusterIDs[sid.ClusterID] = struct{}{}
		}
		for clusterID := range clusterIDs {
			cla := c.getCLA(clusterID, name, p.TargetPort, service.BackendsBySourceID)
			if _, ok := storage[clusterID]; !ok {
				storage[clusterID] = []NamedProtoMessage{}
			}
			storage[clusterID] = append(storage[clusterID], cla)
		}
	}
}

func (c *EndpointCache) getCLA(localClusterID, name string, targetPort int, backendsBySourceID map[k8s.BackendSourceID]k8s.WeightedAddressSet) *api.ClusterLoadAssignment {
	// 1. count scale param for the right incluster per pod weight ratio
	clusterInnerScale := map[k8s.BackendSourceID]int{}
	clusterEPs := map[string][]int{}
	clusterEPsScale := map[string]int{}
	for sourceID, backends := range backendsBySourceID {
		if len(backends.AddrSet) == 0 {
			continue
		}
		if _, ok := clusterEPs[sourceID.ClusterID]; !ok {
			clusterEPs[sourceID.ClusterID] = []int{}
		}
		clusterEPs[sourceID.ClusterID] = append(clusterEPs[sourceID.ClusterID], len(backends.AddrSet))
	}
	for clusterID, epsCount := range clusterEPs {
		clusterEPsScale[clusterID] = LCM(epsCount)
	}
	for sourceID, backends := range backendsBySourceID {
		if len(backends.AddrSet) == 0 {
			continue
		}
		clusterInnerScale[sourceID] = clusterEPsScale[sourceID.ClusterID] / len(backends.AddrSet)
	}

	// 2. count per cluster sum weight
	clusterWeight := map[string]int{}
	for sourceID, backends := range backendsBySourceID {
		if _, ok := clusterInnerScale[sourceID]; !ok {
			continue
		}
		if _, ok := clusterWeight[sourceID.ClusterID]; !ok {
			clusterWeight[sourceID.ClusterID] = 0
		}
		clusterWeight[sourceID.ClusterID] += backends.Weight * clusterInnerScale[sourceID] * len(backends.AddrSet)
	}

	// 3. count per cluster scale param for sum cluster weight aligment
	clusterOuterScale := map[string]int{}
	clusterSumWeights := []int{}
	for _, w := range clusterWeight {
		clusterSumWeights = append(clusterSumWeights, w)
	}
	clusterSumWeight := LCM(clusterSumWeights)
	for clusterID, w := range clusterWeight {
		clusterOuterScale[clusterID] = clusterSumWeight / w
	}

	eps := make([]*endpoint.LocalityLbEndpoints, 0, len(backendsBySourceID))
	for sid, backends := range backendsBySourceID {
		ep := &endpoint.LocalityLbEndpoints{}
		if !c.localityEnabled || sid.ClusterID == localClusterID {
			ep.Priority = 0
		} else {
			ep.Priority = 1
		}

		for _, backend := range backends.AddrSet {
			w := clusterInnerScale[sid] * clusterOuterScale[sid.ClusterID] * backends.Weight
			if w == 0 {
				w = 1
			}
			ep.LbEndpoints = append(
				ep.LbEndpoints,
				lbEndpoint(backend.IP, targetPort, uint32(w)),
			)
		}
		eps = append(eps, ep)
	}

	cla := &api.ClusterLoadAssignment{
		ClusterName: name,
		Endpoints:   eps,
		Policy: &api.ClusterLoadAssignment_Policy{
			OverprovisioningFactor: &wrappers.UInt32Value{Value: 100},
		},
	}

	return cla
}

// lbEndpoint creates a new socketAddress LbEndpoint.
func lbEndpoint(addr string, port int, weight uint32) *endpoint.LbEndpoint {
	return &endpoint.LbEndpoint{
		HostIdentifier: &endpoint.LbEndpoint_Endpoint{
			Endpoint: &endpoint.Endpoint{
				Address: socketAddress(addr, port),
			},
		},
		LoadBalancingWeight: &wrappers.UInt32Value{Value: weight},
	}
}

// GCD finds greatest common divisor (GCD) via Euclidean algorithm
func GCD(a, b int) int {
	for b != 0 {
		t := b
		b = a % b
		a = t
	}
	return a
}

// LCM finds Least Common Multiple (LCM) via GCD
func LCM(numbers []int) int {
	switch len(numbers) {
	case 0:
		return 0
		// case 1:
		// 	return numbers[0]
	}
	a := numbers[0]
	for i := 1; i < len(numbers); i++ {
		b := numbers[i]
		a = a * b / GCD(a, b)
	}
	return a
}
