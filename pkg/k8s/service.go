package k8s

import "fmt"

const (
	WholeNamespaceName = ""
)

type QualifiedName struct {
	Namespace, Name string
}

func NewQualifiedName(namespace, name string) QualifiedName {
	return QualifiedName{Namespace: namespace, Name: name}
}

func (n QualifiedName) IsWholeNamespace() bool {
	return n.Name == WholeNamespaceName
}

func (n QualifiedName) WholeNamespaceKey() QualifiedName {
	wn := n
	wn.Name = WholeNamespaceName
	return wn
}

func (n QualifiedName) String() string {
	name := n.Name
	if name == WholeNamespaceName {
		name = "*"
	}

	return fmt.Sprintf("%s.%s", n.Namespace, name)
}

type Address struct {
	IP, ClusterID string
}

type AddressSet map[string]Address

func (bs AddressSet) isEqual(other AddressSet) bool {
	if len(bs) != len(other) {
		return false
	}

	for i, v := range bs {
		if v != other[i] {
			return false
		}
	}

	return true
}

type WeightedAddressSet struct {
	AddrSet AddressSet
	Weight  int
}

func NewWeightedAddressSet(weight int) WeightedAddressSet {
	return WeightedAddressSet{
		AddrSet: make(AddressSet),
		Weight:  weight,
	}
}

func (was WeightedAddressSet) isEqual(other WeightedAddressSet) bool {
	return was.Weight == other.Weight && was.AddrSet.isEqual(other.AddrSet)
}

func (was WeightedAddressSet) Copy() WeightedAddressSet {
	return WeightedAddressSet{
		AddrSet: was.AddrSet.Copy(),
		Weight:  was.Weight,
	}
}

type BackendSourceID struct {
	EndpointSetName QualifiedName
	ClusterID       string
}

func NewBackendSourceID(endpointSetName QualifiedName, clusterID string) BackendSourceID {
	return BackendSourceID{
		EndpointSetName: endpointSetName,
		ClusterID:       clusterID,
	}
}

func (bs AddressSet) Copy() AddressSet {
	newBackendsSet := make(AddressSet, len(bs))
	for k, v := range bs {
		newBackendsSet[k] = v
	}

	return newBackendsSet
}

type Service struct {
	Namespace, Name string
	// ClusterIPs is a map of cluster IPs across all clusters indexed by cluster ID
	ClusterIPs map[string]Address
	// ClusterIPs is a map of backend IPs set across all clusters indexed by cluster ID
	BackendsBySourceID map[BackendSourceID]WeightedAddressSet
	Ports              []Port
}

func NewService(namespace, name string) *Service {
	return &Service{
		Namespace:          namespace,
		Name:               name,
		ClusterIPs:         map[string]Address{},
		BackendsBySourceID: map[BackendSourceID]WeightedAddressSet{},
		Ports:              []Port{},
	}
}

func (s *Service) UpdateClusterIP(clusterID, ip string) (updated bool) {
	if s.ClusterIPs[clusterID].IP == ip {
		return false
	}

	s.ClusterIPs[clusterID] = Address{IP: ip, ClusterID: clusterID}
	return true
}

func (s *Service) DeleteClusterIP(clusterID string) (updated bool) {
	if _, ok := s.ClusterIPs[clusterID]; !ok {
		return false
	}

	delete(s.ClusterIPs, clusterID)
	return true
}

func (s *Service) GetClusterIP(clusterID string) (ip string, ok bool) {
	addr, ok := s.ClusterIPs[clusterID]
	ip = addr.IP
	return
}

func (s *Service) UpdateBackends(clusterID string, endpointSetName QualifiedName, weight int, ips []string) (updated bool) {
	backendSID := BackendSourceID{
		ClusterID:       clusterID,
		EndpointSetName: endpointSetName,
	}

	newAddresses := NewWeightedAddressSet(weight)
	for _, ip := range ips {
		newAddresses.AddrSet[ip] = Address{ClusterID: clusterID, IP: ip}
	}

	if s.BackendsBySourceID[backendSID].isEqual(newAddresses) {
		return false
	}

	s.BackendsBySourceID[backendSID] = newAddresses
	return true
}

func (s *Service) DeleteBackends(clusterID string, endpointSetName QualifiedName) (updated bool) {
	backendSID := BackendSourceID{
		ClusterID:       clusterID,
		EndpointSetName: endpointSetName,
	}

	if _, ok := s.BackendsBySourceID[backendSID]; !ok {
		return false
	}

	delete(s.BackendsBySourceID, backendSID)
	return true
}

func (s *Service) UpdatePorts(ports []Port) (updated bool) {
	equals := len(s.Ports) == len(ports)

	if equals {
		for i, v := range s.Ports {
			if v != ports[i] {
				equals = false
				break
			}
		}
	}

	if equals {
		return false
	}

	s.Ports = ports

	return true
}

func (s *Service) Copy() *Service {
	newService := *s

	newService.ClusterIPs = AddressSet(s.ClusterIPs).Copy()

	newService.BackendsBySourceID = make(map[BackendSourceID]WeightedAddressSet)
	for k, v := range s.BackendsBySourceID {
		newService.BackendsBySourceID[k] = v.Copy()
	}

	newService.Ports = make([]Port, len(s.Ports))
	copy(newService.Ports, s.Ports)

	return &newService
}

func (s *Service) Key() QualifiedName {
	return NewQualifiedName(s.Namespace, s.Name)
}

// Protocol defines network protocols supported for things like container ports.
type Protocol string

const (
	ProtocolTCP Protocol = "TCP"
)

type Port struct {
	Port, TargetPort int
	Protocol         Protocol
	Name             string
}
