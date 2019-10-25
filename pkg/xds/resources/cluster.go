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
	"crypto/sha256"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	api "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	"github.com/envoyproxy/go-control-plane/envoy/api/v2/core"
	envoyType "github.com/envoyproxy/go-control-plane/envoy/type"
	"github.com/envoyproxy/go-control-plane/pkg/cache"
	"github.com/gogo/protobuf/proto"

	"github.com/avito-tech/navigator/pkg/grpc"
	"github.com/avito-tech/navigator/pkg/k8s"
)

// ClusterCache manages the contents of the gRPC CDS cache.
type ClusterCache struct {
	DynamicConfig

	observable

	mu                    sync.Mutex
	clusterNamesByService map[k8s.QualifiedName][]*api.Cluster
}

func NewClusterCache(opts ...FuncOpt) *ClusterCache {
	c := &ClusterCache{
		clusterNamesByService: map[k8s.QualifiedName][]*api.Cluster{},
	}
	c.DynamicClusterName = DefaultDynamicClusterName
	SetOpts(c, opts...)
	return c
}

func (c *ClusterCache) Get(clusterID string) (resource grpc.Resource, ok bool) {
	// return whole ClusterCache due to envoy clusters shared across all k8s clusters
	return c, true
}

func (c *ClusterCache) AddService(service *k8s.Service) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	serviceKey := service.Key()
	// remove all old 'envoy clusters' of the service
	c.removeService(serviceKey)

	//...and replace them with new
	for _, p := range service.Ports {
		if p.Protocol != k8s.ProtocolTCP {
			continue
		}
		switch getClusterType(p.Name) {
		case TCPCluster:
			name := TCPClusterName(service.Namespace, service.Name, p.Name)
			cluster := newCluster(name, c.DynamicClusterName)
			c.clusterNamesByService[serviceKey] = append(c.clusterNamesByService[serviceKey], cluster)

		case HTTPCluster:
			for backend := range service.BackendsBySourceID {
				name := HTTPClusterName(backend, p.Name)
				cluster := newCluster(name, c.DynamicClusterName)
				c.clusterNamesByService[serviceKey] = append(c.clusterNamesByService[serviceKey], cluster)
			}
		}
	}

	return nil
}

func (c *ClusterCache) RemoveService(serviceKey k8s.QualifiedName) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.removeService(serviceKey)
	return nil
}

func (c *ClusterCache) removeService(serviceKey k8s.QualifiedName) {
	c.clusterNamesByService[serviceKey] = nil
}

func (c *ClusterCache) getNameToClusterMapping() map[string]*api.Cluster {
	mapOfClusters := make(map[string]*api.Cluster)
	for _, v := range c.clusterNamesByService {
		for _, c := range v {
			mapOfClusters[c.Name] = c
		}
	}

	return mapOfClusters
}

func (*ClusterCache) TypeURL() string { return cache.ClusterType }

// Contents returns a copy of the cache's contents.
func (c *ClusterCache) Contents() []proto.Message {
	c.mu.Lock()
	defer c.mu.Unlock()
	var values []proto.Message

	for _, v := range c.getNameToClusterMapping() {
		values = append(values, v)
	}
	sort.Stable(clusterByName(values))
	return values
}

func (c *ClusterCache) Query(names []string) []proto.Message {
	c.mu.Lock()
	defer c.mu.Unlock()

	nameToClusterMapping := c.getNameToClusterMapping()

	var values []proto.Message
	for _, n := range names {
		// if the cluster is not registered we cannot return
		// a blank cluster because each cluster has a required
		// discovery type; DNS, EDS, etc. We cannot determine the
		// correct value for this property from the cluster's name
		// provided by the query so we must not return a blank cluster.
		if v, ok := nameToClusterMapping[n]; ok {
			values = append(values, v)
		}
	}
	sort.Stable(clusterByName(values))
	return values
}

type clusterByName []proto.Message

func (c clusterByName) Len() int           { return len(c) }
func (c clusterByName) Swap(i, j int)      { c[i], c[j] = c[j], c[i] }
func (c clusterByName) Less(i, j int) bool { return c[i].(*api.Cluster).Name < c[j].(*api.Cluster).Name }

// newCluster creates new v2.newCluster from v1.Service.
func newCluster(name, dynamicClusterName string) *api.Cluster {
	edsConfig := configSource(dynamicClusterName)
	return &api.Cluster{
		Name:           name,
		ConnectTimeout: 1 * time.Minute,
		LbPolicy:       api.Cluster_ROUND_ROBIN, // TODO: round robin replacement
		CommonLbConfig: clusterCommonLBConfig(),
		HealthChecks:   nil, // TODO: for now no health checks provided
		EdsClusterConfig: &api.Cluster_EdsClusterConfig{
			EdsConfig:   &edsConfig,
			ServiceName: name,
		},
		ClusterDiscoveryType: &api.Cluster_Type{Type: api.Cluster_EDS},
	}
}

func configSource(cluster string) core.ConfigSource {
	return core.ConfigSource{
		ConfigSourceSpecifier: &core.ConfigSource_ApiConfigSource{
			ApiConfigSource: &core.ApiConfigSource{
				ApiType: core.ApiConfigSource_GRPC,
				GrpcServices: []*core.GrpcService{{
					TargetSpecifier: &core.GrpcService_EnvoyGrpc_{
						EnvoyGrpc: &core.GrpcService_EnvoyGrpc{
							ClusterName: cluster,
						},
					},
				}},
			},
		},
	}
}

// clusterCommonLBConfig creates a *v2.Cluster_CommonLbConfig with HealthyPanicThreshold disabled.
func clusterCommonLBConfig() *api.Cluster_CommonLbConfig {
	return &api.Cluster_CommonLbConfig{
		LocalityConfigSpecifier: &api.Cluster_CommonLbConfig_LocalityWeightedLbConfig_{
			LocalityWeightedLbConfig: &api.Cluster_CommonLbConfig_LocalityWeightedLbConfig{},
		},
		HealthyPanicThreshold: &envoyType.Percent{ // Disable HealthyPanicThreshold
			Value: 0,
		},
	}
}

type ClusterType string

const (
	HTTPCluster ClusterType = "http"
	TCPCluster  ClusterType = "tcp"
)

func getClusterType(portName string) ClusterType {
	tokens := strings.Split(portName, "-")
	if len(tokens) == 0 {
		return TCPCluster
	}
	if tokens[0] == "http" {
		return HTTPCluster
	}
	return TCPCluster
}

// TCPClusterName returns the name of the CDS cluster for this service.
func TCPClusterName(ns, name, portName string) string {
	if portName == "" {
		return hashname(60, ns, name, string(TCPCluster))
	}
	return hashname(60, ns, name, portName, string(TCPCluster))
}

// HTTPClusterName returns the name of the CDS cluster for this service.
func HTTPClusterName(backend k8s.BackendSourceID, portName string) string {
	if portName == "" {
		return hashname(
			60,
			backend.EndpointSetName.Namespace,
			backend.EndpointSetName.Name,
			backend.ClusterID,
			string(HTTPCluster),
		)
	}
	return hashname(
		60,
		backend.EndpointSetName.Namespace,
		backend.EndpointSetName.Name,
		backend.ClusterID,
		portName,
		string(HTTPCluster),
	)
}

// hashname takes a lenth l and a varargs of strings s and returns a string whose length
// which does not exceed l. Internally s is joined with strings.Join(s, "/"). If the
// combined length exceeds l then hashname truncates each element in s, starting from the
// end using a hash derived from the contents of s (not the current element). This process
// continues until the length of s does not exceed l, or all elements have been truncated.
// In which case, the entire string is replaced with a hash not exceeding the length of l.
func hashname(l int, s ...string) string {
	const shorthash = 6 // the length of the shorthash

	r := strings.Join(s, "/")
	if l > len(r) {
		// we're under the limit, nothing to do
		return r
	}
	hash := fmt.Sprintf("%x", sha256.Sum256([]byte(r)))
	for n := len(s) - 1; n >= 0; n-- {
		s[n] = truncate(l/len(s), s[n], hash[:shorthash])
		r = strings.Join(s, "/")
		if l > len(r) {
			return r
		}
	}
	// truncated everything, but we're still too long
	// just return the hash truncated to l.
	return hash[:min(len(hash), l)]
}

// truncate truncates s to l length by replacing the
// end of s with -suffix.
func truncate(l int, s, suffix string) string {
	if l >= len(s) {
		// under the limit, nothing to do
		return s
	}
	if l <= len(suffix) {
		// easy case, just return the start of the suffix
		return suffix[:min(l, len(suffix))]
	}
	return s[:l-len(suffix)-1] + "-" + suffix
}

func min(a, b int) int {
	if a > b {
		return b
	}
	return a
}
