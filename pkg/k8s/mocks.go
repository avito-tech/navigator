package k8s

import (
	"github.com/stretchr/testify/mock"
	"k8s.io/api/extensions/v1beta1"

	navigatorV1 "github.com/avito-tech/navigator/pkg/apis/navigator/v1"
)

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

func (mn *MockNotifier) NotifyNexusesUpdated(updatedAppNames []string, isGateway bool) {
	mn.Called(updatedAppNames, isGateway)
}

func (mn *MockNotifier) NotifyGatewayUpdated(updatedGateway *Gateway, deleted bool) {
	mn.Called(updatedGateway, deleted)
}

func (mn *MockNotifier) NotifyIngressUpdated(updatedIngress *Ingress) {
	mn.Called(updatedIngress)
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

func (mn *MockNexusNotifier) NotifyNexusesUpdated(updatedAppNames []string, isGateway bool) {
	mn.Called(updatedAppNames, isGateway)
}

type MockIngressNotifier struct {
	mock.Mock
}

func (mn *MockIngressNotifier) NotifyIngressUpdated(updatedIngress *Ingress) {
	mn.Called(updatedIngress)
}

type MockGatewayNotifier struct {
	mock.Mock
}

func (mn *MockGatewayNotifier) NotifyGatewayUpdated(updatedGateway *Gateway, deleted bool) {
	mn.Called(updatedGateway, deleted)
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

type MockIngressCache struct {
	mock.Mock
}

func (mic *MockIngressCache) Update(clusterID string, ingress *v1beta1.Ingress, ports map[string]map[int]string) *Ingress {
	args := mic.Called(clusterID, ingress, ports)
	return args.Get(0).(*Ingress)
}

func (mic *MockIngressCache) Remove(clusterID string, ingress *v1beta1.Ingress) *Ingress {
	args := mic.Called(clusterID, ingress)
	return args.Get(0).(*Ingress)
}

func (mic *MockIngressCache) GetByClass(clusterID string, class string) map[QualifiedName]*Ingress {
	args := mic.Called(clusterID, class)
	return args.Get(0).(map[QualifiedName]*Ingress)
}

func (mic *MockIngressCache) GetServicesByClass(clusterID string, class string) []QualifiedName {
	args := mic.Called(clusterID, class)
	return args.Get(0).([]QualifiedName)
}

func (mic *MockIngressCache) GetClusterIDsForIngress(qn QualifiedName, class string) []string {
	args := mic.Called(qn, class)
	return args.Get(0).([]string)
}

func (mic *MockIngressCache) UpdateServicePort(clusterID string, namespace string, name string, ports []Port) []*Ingress {
	args := mic.Called(clusterID, namespace, name, ports)
	return args.Get(0).([]*Ingress)
}

type MockGatewayCache struct {
	mock.Mock
}

func (mgc *MockGatewayCache) Update(clusterID string, gateway *navigatorV1.Gateway) *Gateway {
	args := mgc.Called(clusterID, gateway)
	return args.Get(0).(*Gateway)
}

func (mgc *MockGatewayCache) Delete(clusterID string, gateway *navigatorV1.Gateway) *Gateway {
	args := mgc.Called(clusterID, gateway)
	return args.Get(0).(*Gateway)
}

func (mgc *MockGatewayCache) GatewaysByClass(cluster, ingressClass string) []*Gateway {
	args := mgc.Called(cluster, ingressClass)
	return args.Get(0).([]*Gateway)
}
