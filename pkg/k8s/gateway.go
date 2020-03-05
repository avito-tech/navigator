package k8s

import (
	"sync"

	navigatorV1 "github.com/avito-tech/navigator/pkg/apis/navigator/v1"
)

type GatewayCache interface {
	Update(clusterID string, gateway *navigatorV1.Gateway) *Gateway
	Delete(clusterID string, gateway *navigatorV1.Gateway) *Gateway
	GatewaysByClass(cluster, ingressClass string) []*Gateway
}

// Gateway incapsulates Gateway entity
type Gateway struct {
	Name         string
	ClusterID    string
	IngressClass string
	Port         int
}

type gatewayCache struct {
	// cache contains map with gateway's divided by cluster name
	// map[clusterName][]*Gateway
	cache map[string][]*Gateway

	mu sync.RWMutex
}

func NewGatewayCache() GatewayCache {
	return &gatewayCache{
		cache: map[string][]*Gateway{},
	}
}

func (c *gatewayCache) Update(clusterID string, gateway *navigatorV1.Gateway) *Gateway {
	c.mu.Lock()
	defer c.mu.Unlock()

	gw := &Gateway{
		Name:         gateway.Name,
		ClusterID:    clusterID,
		IngressClass: gateway.Spec.IngressClass,
		Port:         gateway.Spec.Port,
	}

	if _, ok := c.cache[clusterID]; !ok {
		c.cache[clusterID] = []*Gateway{}
	}
	c.cache[clusterID] = append(c.cache[clusterID], gw)

	return gw
}

func (c *gatewayCache) Delete(clusterID string, gateway *navigatorV1.Gateway) *Gateway {
	c.mu.Lock()
	defer c.mu.Unlock()

	gw := &Gateway{
		Name:         gateway.Name,
		ClusterID:    clusterID,
		IngressClass: gateway.Spec.IngressClass,
		Port:         gateway.Spec.Port,
	}

	if _, ok := c.cache[clusterID]; !ok {
		return nil
	}

	for i, cachegw := range c.cache[clusterID] {
		if cachegw.Name == gw.Name {
			// delete i element from slice
			l := len(c.cache[clusterID])
			c.cache[clusterID][i] = c.cache[clusterID][l-1]
			c.cache[clusterID] = c.cache[clusterID][:l-1]
			return gw
		}
	}
	return nil
}

func (c *gatewayCache) GatewaysByClass(clusterID, ingressClass string) []*Gateway {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if _, ok := c.cache[clusterID]; !ok {
		return nil
	}
	var gateways []*Gateway
	for _, gw := range c.cache[clusterID] {
		if gw.IngressClass == ingressClass {
			gateways = append(gateways, gw)
		}
	}
	return gateways
}
