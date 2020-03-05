package resources

import (
	"fmt"
	"sync"

	api "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	core "github.com/envoyproxy/go-control-plane/envoy/api/v2/core"
	listener "github.com/envoyproxy/go-control-plane/envoy/api/v2/listener"
	http "github.com/envoyproxy/go-control-plane/envoy/config/filter/network/http_connection_manager/v2"
	tcp "github.com/envoyproxy/go-control-plane/envoy/config/filter/network/tcp_proxy/v2"
	"github.com/envoyproxy/go-control-plane/pkg/cache"
	"github.com/envoyproxy/go-control-plane/pkg/wellknown"
	"github.com/gogo/protobuf/proto"
	"github.com/golang/protobuf/ptypes"
	"github.com/golang/protobuf/ptypes/any"
	"github.com/golang/protobuf/ptypes/duration"
	"github.com/golang/protobuf/ptypes/wrappers"

	"github.com/avito-tech/navigator/pkg/k8s"
)

const (
	GatewayListenerPrefix = "~~gateway"
)

type ListenerCache struct {
	muUpdate sync.RWMutex

	DynamicConfig
	HealthCheckConfig

	*BranchedResourceCache
	isGatewayOnly bool
}

func NewListenerCache(opts ...FuncOpt) *ListenerCache {
	l := &ListenerCache{
		BranchedResourceCache: NewBranchedResourceCache(cache.ListenerType),
	}

	l.DynamicClusterName = DefaultDynamicClusterName
	SetOpts(l, opts...)

	static := []NamedProtoMessage{}
	if l.EnableHealthCheck {
		static = append(static, l.healthListener())
	}
	l.SetStaticResources(static)
	return l
}

func (c *ListenerCache) UpdateServices(updated, deleted []*k8s.Service) {

	c.muUpdate.Lock()
	defer c.muUpdate.Unlock()

	if c.isGatewayOnly {
		return
	}

	updatedResource := map[string][]NamedProtoMessage{}
	deletedResources := map[string][]NamedProtoMessage{}

	for _, u := range updated {
		c.getServiceListeners(updatedResource, u)
	}
	for _, d := range deleted {
		c.getServiceListeners(deletedResources, d)
	}

	c.UpdateBranchedResources(updatedResource, deletedResources)
}

func (c *ListenerCache) UpdateIngresses(updated map[k8s.QualifiedName]*k8s.Ingress) {
	// do nothing
}

func (c *ListenerCache) UpdateGateway(gw *k8s.Gateway) {
	c.muUpdate.Lock()
	defer c.muUpdate.Unlock()

	if !c.isGatewayOnly {
		c.isGatewayOnly = true
		c.RenewResourceCache()
	}
	updatedResource := map[string][]NamedProtoMessage{}
	c.getGatewayListeners(updatedResource, gw)
	c.UpdateBranchedResources(updatedResource, nil)
}

func (c *ListenerCache) healthListener() *api.Listener {
	return &api.Listener{
		Name:                          c.HealthCheckName,
		Address:                       socketAddress("0.0.0.0", c.HealthCheckPort),
		PerConnectionBufferLimitBytes: &wrappers.UInt32Value{Value: 32768}, // 32 Kb
		FilterChains: []*listener.FilterChain{{
			Filters: []*listener.Filter{{
				Name: wellknown.HTTPConnectionManager,
				ConfigType: &listener.Filter_TypedConfig{
					TypedConfig: makeAny(&http.HttpConnectionManager{
						NormalizePath:     &wrappers.BoolValue{Value: true},
						MergeSlashes:      true,
						GenerateRequestId: &wrappers.BoolValue{Value: false},
						StreamIdleTimeout: &duration.Duration{Seconds: 300},
						StatPrefix:        c.HealthCheckName,
						HttpFilters: []*http.HttpFilter{{
							Name: wellknown.Router,
						}},
						RouteSpecifier: &http.HttpConnectionManager_Rds{
							Rds: &http.Rds{
								RouteConfigName: c.HealthCheckName,
								ConfigSource:    configSource(c.DynamicClusterName),
							},
						},
					}),
				},
			}},
		}},
	}
}

func (c *ListenerCache) getGatewayListeners(storage map[string][]NamedProtoMessage, gateway *k8s.Gateway) {
	clusterID := gateway.ClusterID
	if _, ok := storage[clusterID]; !ok {
		storage[clusterID] = []NamedProtoMessage{}
	}
	listenerConf := gatewayListener("0.0.0.0", gateway.Port, c.DynamicClusterName)
	storage[clusterID] = append(storage[clusterID], listenerConf)
}

func (c *ListenerCache) getServiceListeners(storage map[string][]NamedProtoMessage, service *k8s.Service) {
	for _, p := range service.Ports {
		if p.Protocol != k8s.ProtocolTCP {
			continue
		}
		for _, clusterIP := range service.ClusterIPs {
			clusterID := clusterIP.ClusterID
			var clusterListener *api.Listener
			switch getClusterType(p.Name) {
			case HTTPCluster:
				clusterListener = httpListener(clusterIP, p, service, c.DynamicClusterName)
			case TCPCluster:
				clusterListener = tcpListener(clusterIP, p, service)
			}

			if _, ok := storage[clusterID]; !ok {
				storage[clusterID] = []NamedProtoMessage{}
			}
			storage[clusterID] = append(storage[clusterID], clusterListener)
		}
	}
}

func tcpListener(clusterIP k8s.Address, port k8s.Port, service *k8s.Service) *api.Listener {
	name := ListenerNameFromService(service.Namespace, service.Name, port.Port)

	return &api.Listener{
		Name:    name,
		Address: socketAddress(clusterIP.IP, port.Port),
		DeprecatedV1: &api.Listener_DeprecatedV1{
			BindToPort: &wrappers.BoolValue{
				Value: false,
			},
		},
		FilterChains: []*listener.FilterChain{{
			Filters: []*listener.Filter{{
				Name: wellknown.TCPProxy,
				ConfigType: &listener.Filter_TypedConfig{
					TypedConfig: makeAny(&tcp.TcpProxy{
						StatPrefix: name,
						ClusterSpecifier: &tcp.TcpProxy_Cluster{
							Cluster: ClusterName(service.Namespace, service.Name, port.Name),
						},
					}),
				},
			}},
		}},
	}
}

func httpListener(clusterIP k8s.Address, port k8s.Port, service *k8s.Service, dynamicClusterName string) *api.Listener {
	name := ListenerNameFromService(service.Namespace, service.Name, port.Port)

	return &api.Listener{
		Name:    name,
		Address: socketAddress(clusterIP.IP, port.Port),
		DeprecatedV1: &api.Listener_DeprecatedV1{
			BindToPort: &wrappers.BoolValue{
				Value: false,
			},
		},
		FilterChains: []*listener.FilterChain{{
			Filters: []*listener.Filter{{
				Name: wellknown.HTTPConnectionManager,
				ConfigType: &listener.Filter_TypedConfig{

					TypedConfig: makeAny(&http.HttpConnectionManager{
						NormalizePath: &wrappers.BoolValue{Value: true},
						MergeSlashes:  true,
						UpgradeConfigs: []*http.HttpConnectionManager_UpgradeConfig{{
							UpgradeType: "websocket",
						}},
						StatPrefix: name,
						HttpFilters: []*http.HttpFilter{{
							Name: wellknown.Router,
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

func gatewayListener(address string, port int, dynamicClusterName string) *api.Listener {
	return &api.Listener{
		Name:    fmt.Sprintf("%s_%d", GatewayListenerPrefix, port),
		Address: socketAddress(address, port),
		FilterChains: []*listener.FilterChain{{
			Filters: []*listener.Filter{{
				Name: wellknown.HTTPConnectionManager,
				ConfigType: &listener.Filter_TypedConfig{
					TypedConfig: makeAny(&http.HttpConnectionManager{
						NormalizePath: &wrappers.BoolValue{Value: true},
						MergeSlashes:  true,
						UpgradeConfigs: []*http.HttpConnectionManager_UpgradeConfig{{
							UpgradeType: "websocket",
						}},
						StatPrefix: GatewayListenerPrefix,
						HttpFilters: []*http.HttpFilter{{
							Name: wellknown.Router,
						}},
						RouteSpecifier: &http.HttpConnectionManager_Rds{
							Rds: &http.Rds{
								RouteConfigName: GatewayListenerPrefix,
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
				Protocol: core.SocketAddress_TCP,
				Address:  address,
				PortSpecifier: &core.SocketAddress_PortValue{
					PortValue: uint32(port),
				},
			},
		},
	}
}

func makeAny(pb proto.Message) *any.Any {
	any, err := ptypes.MarshalAny(pb)
	if err != nil {
		panic(err.Error())
	}
	return any
}

func ListenerNameFromService(serviceNamespace, serviceName string, port int) string {
	return fmt.Sprintf("%s_%s_%d", serviceNamespace, serviceName, port)
}
