package k8s

import (
	"sync"
)

type NexusCache interface {
	Delete(appName, version string) (updatedAppNames []string)
	Update(appName, version string, services []QualifiedName) (updatedAppNames []string)
	GetSnapshot() (nexusesByAppName map[string]Nexus, appNamesByService map[QualifiedName][]string)
}

type nexusCache struct {
	mu                sync.RWMutex
	nexusesByAppName  map[string]*partitionedNexus
	appNamesByService map[QualifiedName][]string
}

func NewNexusCache() NexusCache {
	return &nexusCache{
		nexusesByAppName:  make(map[string]*partitionedNexus),
		appNamesByService: make(map[QualifiedName][]string),
	}
}

// Update updates cache if passed dep is different from dep in cache
func (c *nexusCache) Update(appName, partID string, services []QualifiedName) []string {
	c.mu.Lock()
	defer c.mu.Unlock()

	_, ok := c.nexusesByAppName[appName]
	if !ok {
		c.nexusesByAppName[appName] = newPartitionNexus(appName)
	}

	updated := c.nexusesByAppName[appName].UpdateServices(partID, services)
	if !updated {
		return nil
	}

	c.renewAppNamesByService()
	return []string{appName}
}

func (c *nexusCache) Delete(appName, partID string) []string {
	c.mu.Lock()
	defer c.mu.Unlock()

	_, ok := c.nexusesByAppName[appName]
	if !ok {
		return nil
	}

	updated := c.nexusesByAppName[appName].RemoveServices(partID)
	if !updated {
		return nil
	}

	if c.nexusesByAppName[appName].isEmpty() {
		delete(c.nexusesByAppName, appName)
	}

	c.renewAppNamesByService()
	return []string{appName}
}

func (c *nexusCache) renewAppNamesByService() {
	result := make(map[QualifiedName][]string)

	for appName, appPartitionedNexus := range c.nexusesByAppName {
		for _, serviceKey := range appPartitionedNexus.getNexus().Services {
			result[serviceKey] = append(result[serviceKey], appName)
		}
	}

	c.appNamesByService = result
}

func (c *nexusCache) GetSnapshot() (map[string]Nexus, map[QualifiedName][]string) {
	c.mu.RLock()
	nexusesByAppName := make(map[string]Nexus)
	for k, v := range c.nexusesByAppName {
		nexusesByAppName[k] = v.getNexus()
	}
	appNamesByService := make(map[QualifiedName][]string)
	for k, v := range c.appNamesByService {
		appNamesByService[k] = append([]string{}, v...)
	}

	c.mu.RUnlock()
	return nexusesByAppName, appNamesByService
}
