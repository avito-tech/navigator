package k8s

import "github.com/stretchr/testify/mock"

type MockCache struct {
	mock.Mock
}

func (mc *MockCache) UpdateService(namespace, name, clusterID, clusterIP string, ports []Port) []QualifiedName {
	args := mc.Called(namespace, name, clusterID, clusterIP, ports)
	return args.Get(0).([]QualifiedName)
}

func (mc *MockCache) UpdateBackends(serviceName QualifiedName, clusterID string, endpointSetName QualifiedName, weight int, ips []string) []QualifiedName {
	args := mc.Called(serviceName, clusterID, endpointSetName, weight, ips)
	return args.Get(0).([]QualifiedName)
}

func (mc *MockCache) RemoveService(namespace, name string, clusterID string) []QualifiedName {
	args := mc.Called(namespace, name, clusterID)
	return args.Get(0).([]QualifiedName)
}

func (mc *MockCache) RemoveBackends(serviceName QualifiedName, clusterID string, endpointSetName QualifiedName) []QualifiedName {
	args := mc.Called(serviceName, clusterID, endpointSetName)
	return args.Get(0).([]QualifiedName)
}

func (mc *MockCache) FlushServiceByClusterID(serviceName QualifiedName, clusterID string) {
	mc.Called(serviceName, clusterID)
}

func (mc *MockCache) GetSnapshot() map[string]map[string]*Service {
	args := mc.Called()
	return args.Get(0).(map[string]map[string]*Service)
}

type MockCanary struct {
	mock.Mock
}

func (mc *MockCanary) GetMappings(endpointName QualifiedName, clusterID string) map[QualifiedName]EndpointMapping {
	args := mc.Called(endpointName, clusterID)
	return args.Get(0).(map[QualifiedName]EndpointMapping)
}

func (mc *MockCanary) UpdateMapping(serviceName QualifiedName, clusterID string, mappings []EndpointMapping) {
	mc.Called(serviceName, clusterID, mappings)
}

func (mc *MockCanary) DeleteMapping(serviceName QualifiedName, clusterID string) {
	mc.Called(serviceName, clusterID)
}

func (mc *MockCanary) GetDefaultMapping(name QualifiedName) EndpointMapping {
	args := mc.Called(name)
	return args.Get(0).(EndpointMapping)
}

type MockNotifier struct {
	mock.Mock
}

func (mn *MockNotifier) NotifyServicesUpdated(updatedServiceKeys []QualifiedName) {
	mn.Called(updatedServiceKeys)
}

func (mn *MockNotifier) NotifyNexusesUpdated(updatedAppNames []string) {
	mn.Called(updatedAppNames)
}

type MockServiceNotifier struct {
	mock.Mock
}

func (mn *MockServiceNotifier) NotifyServicesUpdated(updatedServiceKeys []QualifiedName) {
	mn.Called(updatedServiceKeys)
}

type MockNexusNotifier struct {
	mock.Mock
}

func (mn *MockNexusNotifier) NotifyNexusesUpdated(updatedAppNames []string) {
	mn.Called(updatedAppNames)
}

type MockServiceEndpointsUpdater struct {
}

type MockNexusCache struct {
	mock.Mock
}

func (mnc *MockNexusCache) Delete(appName, version string) []string {
	args := mnc.Called(appName, version)
	return args.Get(0).([]string)
}

func (mnc *MockNexusCache) Update(appName, version string, services []QualifiedName) []string {
	args := mnc.Called(appName, version, services)
	return args.Get(0).([]string)
}

func (mnc *MockNexusCache) GetSnapshot() (map[string]Nexus, map[QualifiedName][]string) {
	args := mnc.Called()
	return args.Get(0).(map[string]Nexus), args.Get(1).(map[QualifiedName][]string)
}
