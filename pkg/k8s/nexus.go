package k8s

type Nexus struct {
	AppName  string
	Services []QualifiedName //if name is empty, allow access to all services in the namespace
}

func NewNexus(appName string, services []QualifiedName) Nexus {
	return Nexus{
		AppName:  appName,
		Services: append([]QualifiedName{}, services...),
	}
}

func (n Nexus) isEqual(other Nexus) bool {
	if n.AppName != other.AppName {
		return false
	}
	if len(n.Services) != len(other.Services) {
		return false
	}

	//todo: implement set to prevent disorder
	for i, name := range n.Services {
		if name != other.Services[i] {
			return false
		}
	}
	return true
}

// partitionedNexus designed to hold services from multiple Nexus custom resource instances,
// where every instance has unique partition ID (actually, CR Nexus.Name) to correctly process updates/deletes of partitions
// exaplme:
// moment zero: we have Nexus CR {Name: helmgen-v1, services: {test-1, test-2}. resulting navigator nexus has services={test-1, test-2}
// moment 1: appears Nexus CR {Name: helmgen-v2, services: {test-2, test-3}}.  resulting navigator nexus has services={test-1, test-2, test-3}
// moment 2:  Nexus CR "helmgen-v1" DISAPPEARS, still only "helmgen-v2".  resulting navigator nexus has services={test-2, test-3}
type partitionedNexus struct {
	AppName             string
	ServicesByPartition map[string][]QualifiedName
}

func newPartitionNexus(appName string) *partitionedNexus {
	return &partitionedNexus{
		AppName:             appName,
		ServicesByPartition: make(map[string][]QualifiedName),
	}
}

func (pn *partitionedNexus) UpdateServices(partID string, services []QualifiedName) (updated bool) {
	isEqual := NewNexus("", pn.ServicesByPartition[partID]).isEqual(NewNexus("", services))

	//TODO: check equality of result of getNexus() before and after update to detect actual updated/not updated
	if isEqual {
		return false
	}

	pn.ServicesByPartition[partID] = append([]QualifiedName{}, services...)
	return true
}

func (pn *partitionedNexus) RemoveServices(partID string) (updated bool) {
	if _, ok := pn.ServicesByPartition[partID]; !ok {
		return false
	}

	delete(pn.ServicesByPartition, partID)
	return true
}

// getNexus merges services from all partitions into one Nexus
func (pn *partitionedNexus) getNexus() Nexus {
	serviceSet := map[QualifiedName]struct{}{}
	for _, partitionServices := range pn.ServicesByPartition {
		for _, serviceName := range partitionServices {
			serviceSet[serviceName] = struct{}{}
		}
	}

	nexus := NewNexus(pn.AppName, nil)

	for service := range serviceSet {
		nexus.Services = append(nexus.Services, service)
	}

	return nexus
}

func (pn *partitionedNexus) isEmpty() bool {
	return len(pn.ServicesByPartition) == 0
}
