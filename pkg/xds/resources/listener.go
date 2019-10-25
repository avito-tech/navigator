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
	"fmt"
	"sort"
	"sync"

	"github.com/avito-tech/navigator/pkg/grpc"
	"github.com/avito-tech/navigator/pkg/k8s"

	api "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	"github.com/envoyproxy/go-control-plane/envoy/api/v2/core"
	"github.com/envoyproxy/go-control-plane/envoy/api/v2/listener"
	http "github.com/envoyproxy/go-control-plane/envoy/config/filter/network/http_connection_manager/v2"
	tcp "github.com/envoyproxy/go-control-plane/envoy/config/filter/network/tcp_proxy/v2"
	"github.com/envoyproxy/go-control-plane/pkg/cache"
	"github.com/envoyproxy/go-control-plane/pkg/util"
	"github.com/gogo/protobuf/proto"
	"github.com/gogo/protobuf/types"
)

type ListenerByCluster struct {
	ClusterID string
	Listener  *api.Listener
}

// ListenerCache manages the contents of the gRPC LDS cache.
type ListenerCache struct {
	DynamicConfig
	HealthCheckConfig

	observable

	mu                     sync.Mutex
	listenerNamesByService map[k8s.QualifiedName][]ListenerByCluster
}

type clusterListenerCache struct {
	*ListenerCache
	clusterID string
}

// NewListenerCache returns an instance of a ListenerCache
func NewListenerCache(opts ...FuncOpt) *ListenerCache {
	l := &ListenerCache{
		listenerNamesByService: map[k8s.QualifiedName][]ListenerByCluster{},
	}
	l.DynamicClusterName = DefaultDynamicClusterName
	l.EnableHealthCheck = false
	SetOpts(l, opts...)
	return l
}

func (c *ListenerCache) Get(clusterID string) (resource grpc.Resource, ok bool) {
	clustered := &clusterListenerCache{ListenerCache: c, clusterID: clusterID}
	return clustered, true
}

func (c *ListenerCache) AddService(service *k8s.Service) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	serviceKey := service.Key()
	c.removeServiceByName(serviceKey)

	for _, p := range service.Ports {
		if p.Protocol != k8s.ProtocolTCP {
			continue
		}
		for _, clusterIP := range service.ClusterIPs {
			//clusterListeners := c.values[clusterIP.ClusterID]
			//if clusterListeners == nil {
			//	clusterListeners = map[string]*api.Listener{}
			//	c.values[clusterIP.ClusterID] = clusterListeners
			//}

			//name := ListenerName(clusterIP.IP, p.Port)

			var clusterListener *api.Listener
			switch getClusterType(p.Name) {
			case HTTPCluster:
				clusterListener = httpListener(clusterIP, p, c.DynamicClusterName)
			case TCPCluster:
				clusterListener = tcpListener(clusterIP, p, service)
			}

			c.listenerNamesByService[serviceKey] = append(
				c.listenerNamesByService[serviceKey],
				ListenerByCluster{ClusterID: clusterIP.ClusterID, Listener: clusterListener},
			)
		}

	}

	return nil
}

func (c *ListenerCache) RemoveService(serviceKey k8s.QualifiedName) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.removeServiceByName(serviceKey)
	return nil
}

func (c *ListenerCache) removeServiceByName(serviceKey k8s.QualifiedName) {
	c.listenerNamesByService[serviceKey] = nil
}

func (c *clusterListenerCache) getNameToListenersMapping() map[string]map[string]*api.Listener {
	mapping := make(map[string]map[string]*api.Listener)
	for _, v := range c.listenerNamesByService {
		for _, l := range v {
			if _, ok := mapping[l.ClusterID]; !ok {
				mapping[l.ClusterID] = make(map[string]*api.Listener)
			}
			mapping[l.ClusterID][l.Listener.Name] = l.Listener
		}
	}

	return mapping
}

// Contents returns a copy of the cache's contents.
func (c *clusterListenerCache) Contents() []proto.Message {
	c.mu.Lock()
	defer c.mu.Unlock()

	var values []proto.Message
	health := healthListener(c.HealthCheckPort, c.HealthCheckName, c.DynamicClusterName)
	values = append(values, health)
	for _, v := range c.getNameToListenersMapping()[c.clusterID] {
		values = append(values, v)
	}
	sort.Stable(listenersByName(values))
	return values
}

func (c *clusterListenerCache) Query(names []string) []proto.Message {
	c.mu.Lock()
	defer c.mu.Unlock()

	mapping := c.getNameToListenersMapping()

	var values []proto.Message
	for _, n := range names {
		v, ok := mapping[c.clusterID][n]
		if ok {
			values = append(values, v)
		}
		if n == c.HealthCheckName {
			health := healthListener(c.HealthCheckPort, c.HealthCheckName, c.DynamicClusterName)
			values = append(values, health)
		}
	}
	sort.Stable(listenersByName(values))
	return values
}

func (*ListenerCache) TypeURL() string { return cache.ListenerType }

type listenersByName []proto.Message

func (l listenersByName) Len() int      { return len(l) }
func (l listenersByName) Swap(i, j int) { l[i], l[j] = l[j], l[i] }
func (l listenersByName) Less(i, j int) bool {
	return l[i].(*api.Listener).Name < l[j].(*api.Listener).Name
}

func tcpListener(clusterIP k8s.Address, port k8s.Port, service *k8s.Service) *api.Listener {
	name := ListenerName(clusterIP.IP, port.Port)

	return &api.Listener{
		Name:    name,
		Address: *socketAddress(clusterIP.IP, port.Port),
		DeprecatedV1: &api.Listener_DeprecatedV1{
			BindToPort: &types.BoolValue{
				Value: false,
			},
		},
		FilterChains: []listener.FilterChain{{
			Filters: []listener.Filter{{
				Name: util.TCPProxy,
				ConfigType: &listener.Filter_TypedConfig{
					TypedConfig: any(&tcp.TcpProxy{
						StatPrefix: name,
						ClusterSpecifier: &tcp.TcpProxy_Cluster{
							Cluster: TCPClusterName(service.Namespace, service.Name, port.Name),
						},
					}),
				},
			}},
		}},
	}
}

func httpListener(clusterIP k8s.Address, port k8s.Port, dynamicClusterName string) *api.Listener {
	name := ListenerName(clusterIP.IP, port.Port)

	return &api.Listener{
		Name:    name,
		Address: *socketAddress(clusterIP.IP, port.Port),
		DeprecatedV1: &api.Listener_DeprecatedV1{
			BindToPort: &types.BoolValue{
				Value: false,
			},
		},
		FilterChains: []listener.FilterChain{{
			Filters: []listener.Filter{{
				Name: util.HTTPConnectionManager,
				ConfigType: &listener.Filter_TypedConfig{

					TypedConfig: any(&http.HttpConnectionManager{
						StatPrefix: name,
						HttpFilters: []*http.HttpFilter{{
							Name: util.Router,
						}},
						RouteSpecifier: &http.HttpConnectionManager_Rds{
							Rds: &http.Rds{
								RouteConfigName: name,
								ConfigSource:    configSource(dynamicClusterName),
							},
						},
					}),
				},
			}},
		}},
	}
}

func healthListener(port int, name, dynamicClusterName string) *api.Listener {
	return &api.Listener{
		Name:    name,
		Address: *socketAddress("0.0.0.0", port),
		FilterChains: []listener.FilterChain{{
			Filters: []listener.Filter{{
				Name: util.HTTPConnectionManager,
				ConfigType: &listener.Filter_TypedConfig{

					TypedConfig: any(&http.HttpConnectionManager{
						StatPrefix: name,
						HttpFilters: []*http.HttpFilter{{
							Name: util.Router,
						}},
						RouteSpecifier: &http.HttpConnectionManager_Rds{
							Rds: &http.Rds{
								RouteConfigName: name,
								ConfigSource:    configSource(dynamicClusterName),
							},
						},
					}),
				},
			}},
		}},
	}

}

// socketAddress creates a new TCP core.Address.
func socketAddress(address string, port int) *core.Address {
	return &core.Address{
		Address: &core.Address_SocketAddress{
			SocketAddress: &core.SocketAddress{
				Protocol: core.TCP,
				Address:  address,
				PortSpecifier: &core.SocketAddress_PortValue{
					PortValue: uint32(port),
				},
			},
		},
	}
}

func any(pb proto.Message) *types.Any {
	any, err := types.MarshalAny(pb)
	if err != nil {
		panic(err.Error())
	}
	return any
}

func ListenerName(ip string, port int) string {
	return fmt.Sprintf("%s_%d", ip, port)
}
