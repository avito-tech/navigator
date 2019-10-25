// Copyright © 2018 Heptio
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package resources

import (
	"sort"
	"sync"

	api "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	"github.com/envoyproxy/go-control-plane/envoy/api/v2/endpoint"
	"github.com/envoyproxy/go-control-plane/pkg/cache"
	"github.com/gogo/protobuf/proto"
	"github.com/gogo/protobuf/types"

	"github.com/avito-tech/navigator/pkg/grpc"
	"github.com/avito-tech/navigator/pkg/k8s"
)

type EndpointCache struct {
	observable

	mu                    sync.Mutex
	clusterNamesByService map[k8s.QualifiedName][]*api.ClusterLoadAssignment
}

func NewEndpointCache(opts ...FuncOpt) *EndpointCache {
	e := &EndpointCache{
		clusterNamesByService: map[k8s.QualifiedName][]*api.ClusterLoadAssignment{},
	}
	SetOpts(e, opts...)
	return e
}

func (c *EndpointCache) Get(clusterID string) (resource grpc.Resource, ok bool) {
	// return whole EndpointCache due to envoy clusters shared across all k8s clusters
	return c, true
}

func (c *EndpointCache) AddService(service *k8s.Service) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	serviceKey := service.Key()
	c.removeServiceByName(serviceKey)

	for _, p := range service.Ports {
		if p.Protocol != k8s.ProtocolTCP {
			continue
		}

		switch getClusterType(p.Name) {
		case TCPCluster:
			name := TCPClusterName(service.Namespace, service.Name, p.Name)
			cla := getCLA(name, p.TargetPort, service.BackendsBySourceID)
			c.clusterNamesByService[serviceKey] = append(c.clusterNamesByService[serviceKey], cla)

		case HTTPCluster:
			for backend, backendsBySourceID := range service.BackendsBySourceID {
				name := HTTPClusterName(backend, p.Name)
				cla := getCLA(name, p.TargetPort, map[k8s.BackendSourceID]k8s.WeightedAddressSet{backend: backendsBySourceID})
				c.clusterNamesByService[serviceKey] = append(c.clusterNamesByService[serviceKey], cla)
			}
		}
	}

	return nil
}

func getCLA(name string, targetPort int, backendsBySourceID map[k8s.BackendSourceID]k8s.WeightedAddressSet) *api.ClusterLoadAssignment {
	cla := &api.ClusterLoadAssignment{
		ClusterName: name,
		Endpoints:   make([]endpoint.LocalityLbEndpoints, len(backendsBySourceID)),
	}

	// clusterID -> multiply coeff to support equal final sum in each cluster
	balancingCoeffs := make(map[string]float32)
	for sourceID, backends := range backendsBySourceID {
		balancingCoeffs[sourceID.ClusterID] += float32(backends.Weight)
	}
	for k, sum := range balancingCoeffs {
		balancingCoeffs[k] = float32(k8s.EndpointsWeightSumForCluster) / sum
	}

	i := 0
	for _, backends := range backendsBySourceID {
		cla.Endpoints[i].LoadBalancingWeight = &types.UInt32Value{Value: uint32(backends.Weight)}
		//TODO: add locality here
		for _, backend := range backends.AddrSet {
			// TODO: compute locality
			//cla.Endpoints[0].Locality = backend.ClusterID
			cla.Endpoints[i].LbEndpoints = append(cla.Endpoints[i].LbEndpoints,
				lbEndpoint(
					backend.IP,
					targetPort,
					uint32(
						(float32(backends.Weight)/float32(len(backends.AddrSet)))*balancingCoeffs[backend.ClusterID])))
		}
		i++
	}
	return cla
}

func (c *EndpointCache) RemoveService(serviceKey k8s.QualifiedName) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.removeServiceByName(serviceKey)

	return nil
}

func (c *EndpointCache) removeServiceByName(serviceKey k8s.QualifiedName) {
	if c.clusterNamesByService == nil {
		c.clusterNamesByService = make(map[k8s.QualifiedName][]*api.ClusterLoadAssignment)
	}

	c.clusterNamesByService[serviceKey] = nil
}

func (c *EndpointCache) getNameToCLAMapping() map[string]*api.ClusterLoadAssignment {
	mapOfCLA := make(map[string]*api.ClusterLoadAssignment)
	for _, v := range c.clusterNamesByService {
		for _, c := range v {
			mapOfCLA[c.ClusterName] = c
		}
	}

	return mapOfCLA
}

// Contents returns a copy of the contents of the cache.
func (c *EndpointCache) Contents() []proto.Message {
	c.mu.Lock()
	defer c.mu.Unlock()
	var values []proto.Message
	for _, v := range c.getNameToCLAMapping() {
		values = append(values, v)
	}

	sort.Stable(clusterLoadAssignmentsByName(values))
	return values
}

func (c *EndpointCache) Query(names []string) []proto.Message {
	c.mu.Lock()
	defer c.mu.Unlock()
	mapOfCLAs := c.getNameToCLAMapping()
	var values []proto.Message
	for _, n := range names {
		v, ok := mapOfCLAs[n]
		if !ok {
			v = &api.ClusterLoadAssignment{
				ClusterName: n,
			}
		}
		values = append(values, v)
	}
	sort.Stable(clusterLoadAssignmentsByName(values))
	return values
}

func (*EndpointCache) TypeURL() string { return cache.EndpointType }

type clusterLoadAssignmentsByName []proto.Message

func (c clusterLoadAssignmentsByName) Len() int      { return len(c) }
func (c clusterLoadAssignmentsByName) Swap(i, j int) { c[i], c[j] = c[j], c[i] }
func (c clusterLoadAssignmentsByName) Less(i, j int) bool {
	return c[i].(*api.ClusterLoadAssignment).ClusterName < c[j].(*api.ClusterLoadAssignment).ClusterName
}

// lbEndpoint creates a new socketAddress LbEndpoint.
func lbEndpoint(addr string, port int, weight uint32) endpoint.LbEndpoint {
	return endpoint.LbEndpoint{
		HostIdentifier: &endpoint.LbEndpoint_Endpoint{
			Endpoint: &endpoint.Endpoint{
				Address: socketAddress(addr, port),
			},
		},
		LoadBalancingWeight: &types.UInt32Value{Value: weight},
	}
}
