package resources

import (
	"fmt"
	"sort"
	"sync"

	"github.com/avito-tech/navigator/pkg/grpc"
	api "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	"github.com/gogo/protobuf/proto"
)

type NamedProtoMessage interface {
	proto.Message
}

// BranchedResourceCache implements ResourceCache for some resource space
// it is used to assign resources to k8s clusters
// thit name is chosen in order to avoid confusion with envoy's cluster resources
type BranchedResourceCache struct {
	mu sync.RWMutex

	// cache contains resources for each bra
	cache map[string]*resourceCache

	staticResources []NamedProtoMessage

	typeURL string
}

func NewBranchedResourceCache(typeURL string) *BranchedResourceCache {
	return &BranchedResourceCache{
		cache:           map[string]*resourceCache{},
		staticResources: []NamedProtoMessage{},
		typeURL:         typeURL,
	}
}

func (c *BranchedResourceCache) Get(branch string) (grpc.Resource, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	r, ok := c.cache[branch]
	if !ok {
		r = NewResourceCache(c.typeURL, c.staticResources)
		c.cache[branch] = r
	}
	return r, true
}

func (c *BranchedResourceCache) SetStaticResources(staticResources []NamedProtoMessage) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.staticResources = staticResources
}

// UpdateBranchedResources makes received changes in ResourceCache
func (c *BranchedResourceCache) UpdateBranchedResources(updated, deleted map[string][]NamedProtoMessage) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// update ResourceCache with new or updated resources
	for branch, resources := range updated {
		c.renewResourceCache(branch)
		if _, ok := c.cache[branch]; !ok {
			c.cache[branch] = NewResourceCache(c.typeURL, c.staticResources)
		}
		c.cache[branch].UpdateResources(resources)
	}

	// 2. delete resources from ResourceCache
	for branch, resources := range deleted {
		if _, ok := c.cache[branch]; !ok {
			continue
		}
		c.cache[branch].DeleteResources(resources)
	}
}

func (c *BranchedResourceCache) RenewResourceCache(branches ...string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.renewResourceCache(branches...)
}

func (c *BranchedResourceCache) renewResourceCache(branches ...string) {
	for _, branch := range branches {
		if resources, ok := c.cache[branch]; ok {
			resources.Flush()
			resources.UpdateResources(c.staticResources)
		}
	}
}

func (c *BranchedResourceCache) NotifySubscribers() {
	c.mu.RLock()
	defer c.mu.RUnlock()

	for _, cache := range c.cache {
		cache.NotifySubscribers()
	}
}

func (c *BranchedResourceCache) TypeURL() string {
	return c.typeURL
}

func (c *BranchedResourceCache) String() string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	var result string
	for cluster, resources := range c.cache {
		result += fmt.Sprintf(">>> Cluster %s \n", cluster)
		for resname := range resources.cache {
			result += fmt.Sprintf("+ %s \n", resname)
		}
	}
	return result
}

type resourceCache struct {
	mu sync.RWMutex
	// map[resourceName]resource
	cache   map[string]NamedProtoMessage
	typeURL string

	last    int
	waiters map[string]chan<- grpc.PushEvent
}

func NewResourceCache(typeURL string, initResources []NamedProtoMessage) *resourceCache {
	r := &resourceCache{
		cache:   map[string]NamedProtoMessage{},
		typeURL: typeURL,
		waiters: make(map[string]chan<- grpc.PushEvent),
	}
	r.UpdateResources(initResources)
	return r
}

// UpdateResources sets resources in cache with old values overwriting
func (c *resourceCache) UpdateResources(resources []NamedProtoMessage) {
	c.mu.Lock()
	defer c.mu.Unlock()

	for _, res := range resources {
		name := GetResourceName(res)
		c.cache[name] = res
	}
}

// DeleteResources removes resources from cache
// actually, only names of resources are needed
func (c *resourceCache) DeleteResources(resources []NamedProtoMessage) {
	c.mu.Lock()
	defer c.mu.Unlock()

	for _, r := range resources {
		name := GetResourceName(r)
		if _, ok := c.cache[name]; ok {
			delete(c.cache, name)
		}
	}
}

func (c *resourceCache) Flush() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.cache = map[string]NamedProtoMessage{}
}

// IsEmpty checks if cache is empty
func (c *resourceCache) IsEmpty() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return len(c.cache) == 0
}

// Contents returns a copy of the cache's contents
func (c *resourceCache) Contents() ([]proto.Message, []string) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	names := make([]string, 0, len(c.cache))
	values := []proto.Message{}
	for name, r := range c.cache {
		names = append(names, name)
		values = append(values, r)
	}
	sort.Stable(resourcesByName(values))
	return values, names
}

// Query returns requested resources
func (c *resourceCache) Query(names []string) []proto.Message {
	c.mu.RLock()
	defer c.mu.RUnlock()

	values := []proto.Message{}
	for _, n := range names {
		if r, ok := c.cache[n]; ok {
			values = append(values, r)
		}
	}
	sort.Stable(resourcesByName(values))
	return values
}

func (c *resourceCache) TypeURL() string {
	return c.typeURL
}

// func (c *resourceCache) Register(ch chan int, last int) {
// 	c.mu.Lock()
// 	defer c.mu.Unlock()

// 	if last < c.last {
// 		// notify this channel immediately
// 		ch <- c.last
// 		return
// 	}
// 	c.waiters = append(c.waiters, ch)
// }

// notify notifies all registered waiters that an event has occurred.

type pushEvent struct {
	nonce   int
	typeURL string
}

func (p *pushEvent) Nonce() int {
	return p.nonce
}

func (p *pushEvent) ResourceType() string {
	return p.typeURL
}

func (c *resourceCache) NotifySubscribers() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.last++

	for _, ch := range c.waiters {
		ch <- &pushEvent{
			nonce:   c.last,
			typeURL: c.typeURL,
		}
	}
	c.waiters = make(map[string]chan<- grpc.PushEvent, len(c.waiters))
}

func (c *resourceCache) Nonce() int {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return c.last
}

func (c *resourceCache) Unregister(name string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	delete(c.waiters, name)
}

func (c *resourceCache) Register(name string, pushCh chan<- grpc.PushEvent) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.waiters[name] = pushCh
}

type resourcesByName []proto.Message

func (l resourcesByName) Len() int      { return len(l) }
func (l resourcesByName) Swap(i, j int) { l[i], l[j] = l[j], l[i] }
func (l resourcesByName) Less(i, j int) bool {
	return GetResourceName(l[i]) < GetResourceName(l[j])
}

// GetResourceName returns the resource name for a valid xDS response type.
func GetResourceName(res proto.Message) string {
	switch v := res.(type) {
	case *api.ClusterLoadAssignment:
		return v.GetClusterName()
	case *api.Cluster:
		return v.GetName()
	case *api.RouteConfiguration:
		return v.GetName()
	case *api.Listener:
		return v.GetName()
	default:
		return ""
	}
}
