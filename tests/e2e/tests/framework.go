package tests

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"os"
	"os/exec"
	"sort"
	"strings"
	"sync"
	"time"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/avito-tech/navigator/pkg/apis/generated/clientset/versioned"
	v12 "github.com/avito-tech/navigator/pkg/apis/navigator/v1"
	"github.com/avito-tech/navigator/pkg/k8s"
)

const (
	podWaitCheckInterval = 1 * time.Second
	WholeNamespace       = k8s.WholeNamespaceName
	NavigatorNamespace   = "navigator-system"
	clusterPrefix        = "cluster-"
	appPrefix            = "test-"
)

type TestEnv struct {
	clusterNames []string
}

func Setup(timeout time.Duration, clusterCount int, appList []string, enableLocality bool) error {
	log.Printf("Deploy services...\n")
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	enableLocalityString := map[bool]string{true: "true", false: "false"}[enableLocality]
	out, err := runScript(
		ctx,
		clusterCount,
		appList,
		map[string]string{"ENABLE_LOCALITY": enableLocalityString},
		"../setup/deploy.sh",
	)
	if err != nil {
		log.Printf("Deploy services:\n%s\n", out)
		return fmt.Errorf("failed to deploy services: %s", err.Error())
	}

	log.Printf("Services successfully deployed!\n")
	return nil
}

func Clean(timeout time.Duration, clusterCount int, appList []string) error {
	log.Printf("Cleaning services...\n")
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	out, err := runScript(ctx, clusterCount, appList, nil, "../setup/clean.sh")
	if err != nil {
		log.Printf("\n%s\n=================\n", out)
		return fmt.Errorf("failed to clean services: %s", err.Error())
	}

	log.Printf("Services successfully cleaned!\n")
	return nil
}

func UpdateDiscovery(clusterCount int, discoveries []Discovery) error {
	var deps []*v12.Nexus
	for v, d := range discoveries {
		deps = append(deps, d.MarshalCr(v)...)

	}
	for _, clusterName := range getClusterList(clusterCount) {
		err := UpdateNexuses(clusterName, deps)
		if err != nil {
			return fmt.Errorf("failed to update discovery in cluster %q: %s", clusterName, err.Error())
		}
	}

	return nil
}

func runScript(ctx context.Context, clusterCount int, appList []string, env map[string]string, scriptPath string) (out []byte, err error) {
	cmd := exec.CommandContext(ctx, "bash", scriptPath)
	additionalEnv := []string{
		fmt.Sprintf("CLUSTER_COUNT=%d", clusterCount),
		fmt.Sprintf("NS_LIST=%s", strings.Join(appList, " ")),
	}

	for k, v := range env {
		additionalEnv = append(additionalEnv, fmt.Sprintf(fmt.Sprintf("%s=%s", k, v)))
	}

	cmd.Env = append(os.Environ(), additionalEnv...)
	return cmd.CombinedOutput()
}

type Discovery map[string]map[string]map[string]bool

func NewDiscovery() Discovery {
	return make(Discovery)
}

func (d Discovery) Copy() Discovery {
	newDiscovery := NewDiscovery()

	for srcApp, destinations := range d {
		for dstNamespace, components := range destinations {
			for dstService, accessibility := range components {
				newDiscovery.Add(srcApp, dstNamespace, dstService, accessibility)
			}
		}
	}

	return newDiscovery
}

func (d *Discovery) MarshalCr(version int) (nexuses []*v12.Nexus) {
	for srcAppName, destinations := range *d {
		dep := &v12.Nexus{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: NavigatorNamespace,
				Name:      fmt.Sprintf("%s-v%d", srcAppName, version),
			},
			Spec: v12.NexusSpec{
				AppName:  srcAppName,
				Services: []v12.Service{},
			},
		}
		nexuses = append(nexuses, dep)

		for dstApp, dstComponents := range destinations {
			for dstComponent, isAccessible := range dstComponents {
				if !isAccessible {
					continue
				}

				dep.Spec.Services = append(
					dep.Spec.Services,
					v12.Service{Namespace: dstApp, Name: dstComponent},
				)
			}
		}
	}

	return nexuses
}

func (d Discovery) IsAccessible(srcAppName, dstNamespace, dstServiceName string) bool {
	if accessibility, ok := d[srcAppName][dstNamespace][dstServiceName]; ok {
		return accessibility
	}
	return d[srcAppName][dstNamespace][WholeNamespace]
}

// DeclareExistingDestinationAccessibility changes certain component accessibility for all already accessible namespaces
func (d Discovery) DeclareExistingDestinationAccessibility(dstNamespace, dstComponentName string, accessible bool) {
	for _, destinations := range d {
		// change component accessibility if only whole destination NS is ACCESSIBLE
		if destinations[dstNamespace][WholeNamespace] {
			destinations[dstNamespace][dstComponentName] = accessible
		}
	}
}

func (d Discovery) Add(srcAppName, dstNamespace, dstServiceName string, accessible bool) {
	if d[srcAppName] == nil {
		d[srcAppName] = make(map[string]map[string]bool)
	}
	if d[srcAppName][dstNamespace] == nil {
		d[srcAppName][dstNamespace] = make(map[string]bool)
	}

	d[srcAppName][dstNamespace][dstServiceName] = accessible
}

func (d Discovery) RemoveDestination(srcAppName, dstNamespace string) {
	delete(d[srcAppName], dstNamespace)
}

func (d Discovery) RemoveSource(srcAppName string) {
	delete(d, srcAppName)
}

func (d Discovery) FlushDestinations(srcAppName string) {
	d[srcAppName] = make(map[string]map[string]bool)
}

func (d Discovery) String() string {
	//str, _ := json.MarshalIndent(d, "", "  ")
	//return string(str)

	str := ""

	for srcApp, destinations := range d {
		var result []string
		for dstNamespace, components := range destinations {
			for dstService, accessibility := range components {
				if dstService == WholeNamespace {
					dstService = "*"
				}
				accessSign := map[bool]string{true: "->", false: "><"}[accessibility]
				result = append(result, fmt.Sprintf("%s  %s  %s.%s", srcApp, accessSign, dstNamespace, dstService))
			}
		}
		sort.Strings(result)
		str += strings.Join(result, "\n") + "\n\n"
	}

	return str
}

type ComponentKey struct {
	AppName       string
	ComponentName string
}

func (c ComponentKey) String() string {
	return fmt.Sprintf("%s.%s", c.AppName, c.ComponentName)
}

type Response struct {
	Pod     string
	Cluster string
}

type ComponentStat struct {
	mu sync.RWMutex

	ComponentKey
	ResponsesByCluster map[string]map[string]int
	Durations          map[string][]string
	InvalidResponses   []string
	Fails              map[string]int
	FailsCount         int
	SuccessCount       int
	TotalProbes        int
}

func (s *ComponentStat) AddFail(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.Fails == nil {
		s.Fails = make(map[string]int)
	}

	s.FailsCount++
	s.Fails[err.Error()]++
}

func (s *ComponentStat) AddResponse(responseString string, duration time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var response Response
	err := json.Unmarshal([]byte(responseString), &response)
	if err != nil {
		s.InvalidResponses = append(s.InvalidResponses, responseString)
		return
	}

	if s.Durations == nil {
		s.Durations = make(map[string][]string)
	}

	if s.ResponsesByCluster == nil {
		s.ResponsesByCluster = map[string]map[string]int{}
	}

	if s.ResponsesByCluster[response.Cluster] == nil {
		s.ResponsesByCluster[response.Cluster] = map[string]int{}
	}

	//s.Durations[podName] = append(s.Durations[podName] , fmt.Sprintf("%d ms", duration.Nanoseconds() / 1000000))
	s.ResponsesByCluster[response.Cluster][response.Pod]++
	s.SuccessCount++
}

func (s *ComponentStat) IsInvalid() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return len(s.InvalidResponses) > 0
}

func (s *ComponentStat) IsUnstable() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.FailsCount != s.TotalProbes && s.FailsCount != 0
}

func (s *ComponentStat) IsAccessible(wantPodsWeightsByCluster map[string]map[string]int) (accessible bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for cluster, wantPods := range wantPodsWeightsByCluster {
		for wantPod := range wantPods {
			if _, ok := s.ResponsesByCluster[cluster][wantPod]; ok {
				return true
			}
		}
	}

	return false
}

func (s *ComponentStat) IsAllBackendsAccessible(wantPodsWeightsByCluster map[string]map[string]int) (fullyBalanced bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	fullyBalanced = true
	for cluster, wantPods := range wantPodsWeightsByCluster {
		if len(wantPods) != len(s.ResponsesByCluster[cluster]) {
			return false
		}
		for wantPod := range wantPods {
			if _, ok := s.ResponsesByCluster[cluster][wantPod]; !ok {
				return false
			}
		}
	}

	return true
}

func (s *ComponentStat) CheckBalancingWeighted(wantPodsWeightsByCluster map[string]map[string]int, wantBiasRatio float64) error {
	wantNormalizedWeights := getNormalizedWeights(wantPodsWeightsByCluster, s.TotalProbes)
	gotNormalizedWeights := getNormalizedWeights(s.ResponsesByCluster, s.TotalProbes)

	for clusterID, podWeights := range wantNormalizedWeights {
		for podName, wantWeight := range podWeights {
			biasRatio := math.Abs(float64(gotNormalizedWeights[clusterID][podName])-float64(wantWeight)) / float64(wantWeight)
			if biasRatio > wantBiasRatio {
				prettyWantPods, _ := json.MarshalIndent(wantNormalizedWeights, "", "  ")
				return fmt.Errorf(
					"destination pod %s.%s: bias ratio %.3f > %.3f. got weight ratio %d, want %d.\n Want pod weights:\n%s\n",
					clusterID, podName, biasRatio, wantBiasRatio, gotNormalizedWeights[clusterID][podName], wantWeight, prettyWantPods,
				)
			}
		}
	}

	return nil
}

func getNormalizedWeights(podsWeightsByCluster map[string]map[string]int, totalProbes int) map[string]map[string]int {
	totalWeight := 0
	for _, podWeights := range podsWeightsByCluster {
		for _, weight := range podWeights {
			totalWeight += weight
		}
	}

	normalizedWeights := map[string]map[string]int{}
	for clusterID, podWeights := range podsWeightsByCluster {
		normalizedWeights[clusterID] = map[string]int{}
		for podName, weight := range podWeights {
			normalizedWeights[clusterID][podName] = int(math.Round(float64(totalProbes) * float64(weight) / float64(totalWeight)))
		}
	}

	return normalizedWeights
}

func getAppStats(ctx context.Context, clusterName, appName string, appList, componentList []string, port, count int, componentsProtos map[string]string, uri string) (stats map[string]ComponentStat, err error) {
	componentProtosChunks := []string{}
	for c, p := range componentsProtos {
		componentProtosChunks = append(componentProtosChunks, fmt.Sprintf("%s:%s", c, p))
	}
	componentProtosString := strings.Join(componentProtosChunks, "&componentsProtos=")
	appListString := strings.Join(appList, "&appList=")
	componentListString := strings.Join(componentList, "&componentList=")
	url := fmt.Sprintf("http://127.0.0.1?count=%d&appList=%s&componentList=%s&componentsProtos=%s&probePort=%d&url=%s",
		count,
		appListString,
		componentListString,
		componentProtosString,
		port,
		uri,
	)
	cmd := exec.CommandContext(
		ctx, "bash", "../setup/exec.sh",
		"-n", appName, "exec", "-c", "c", "curl", "--", "curl", "-s",
		url,
	)
	additionalEnv := []string{
		fmt.Sprintf("CLUSTER=%s", clusterName),
	}

	cmd.Env = append(os.Environ(), additionalEnv...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("failed to fetch appstats clusterName=%q app=%q: %q", clusterName, appName, err.Error())
	}

	err = json.Unmarshal(out, &stats)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal appstats clusterName=%q app=%q: %q\n---\n%s\n---", clusterName, appName, err.Error(), out)
	}

	return stats, nil
}

func getLogs(ctx context.Context, clusterName, namespace, podName string) (logs string, err error) {
	cmd := exec.CommandContext(
		ctx, "bash", "../setup/exec.sh", "logs", "-n", namespace, podName,
	)
	additionalEnv := []string{
		fmt.Sprintf("CLUSTER=%s", clusterName),
	}

	cmd.Env = append(os.Environ(), additionalEnv...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("failed to get logs clusterName=%q %q.%q: %q", clusterName, namespace, podName, err.Error())
	}

	return string(out), nil
}

func getClientsetByClusterName(cluster string) (*kubernetes.Clientset, error) {
	cmd := exec.Command("kind", "get", "kubeconfig-path", fmt.Sprintf("--name=%s", cluster))
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("failed to get kind kubeconfig for cluster %q: %s", cluster, err.Error())
	}

	kubeconfigPath := strings.TrimSpace(string(out))

	config, err := clientcmd.BuildConfigFromFlags("", kubeconfigPath)
	if err != nil {
		return nil, fmt.Errorf("failed to build clientset for cluster %q: %s", cluster, err.Error())
	}

	client, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to build clientset for cluster %q: %s", cluster, err.Error())
	}
	return client, nil
}

func getExtClientsetByClusterName(cluster string) (*versioned.Clientset, error) {
	cmd := exec.Command("kind", "get", "kubeconfig-path", fmt.Sprintf("--name=%s", cluster))
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("failed to get kind kubeconfig for cluster %q: %s", cluster, err.Error())
	}

	kubeconfigPath := strings.TrimSpace(string(out))

	config, err := clientcmd.BuildConfigFromFlags("", kubeconfigPath)
	if err != nil {
		return nil, err
	}

	client, err := versioned.NewForConfig(config)
	return client, err
}

func GetPodNames(clusterName, appName, componentName string) (podNames []string, err error) {
	cs, err := getClientsetByClusterName(clusterName)
	if err != nil {
		return nil, err
	}

	podList, err := cs.CoreV1().Pods(appName).List(metav1.ListOptions{LabelSelector: fmt.Sprintf("component=%s", componentName)})
	if err != nil {
		return nil, fmt.Errorf("failed to list pods cluster=%q app=%q: %s", clusterName, appName, err.Error())
	}

	for _, pod := range podList.Items {
		podNames = append(podNames, pod.Name)
		//podNames = append(podNames, fmt.Sprintf("%s.%s.%s", ClusterName, appName, pod.Name))
	}

	return
}

func ScaleReplicaset(timeout time.Duration, clusterName, appName, componentName string, replicas int32) error {
	cs, err := getClientsetByClusterName(clusterName)
	if err != nil {
		return err
	}

	rsName := fmt.Sprintf("%s.%s", appName, componentName)

	d, err := cs.AppsV1().ReplicaSets(appName).Get(rsName, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("failed to get deploy %q.%q in cluster %q: %s", appName, componentName, clusterName, err.Error())
	}

	d.Spec.Replicas = &replicas
	_, err = cs.AppsV1().ReplicaSets(appName).Update(d)

	if err != nil {
		return fmt.Errorf(
			"failed to scale deploy %q.%q in cluster %q to replicas=%d: %s",
			clusterName,
			appName,
			componentName,
			replicas,
			err.Error(),
		)
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	err = WaitPodsStarted(ctx, clusterName, appName)
	if err != nil {
		return fmt.Errorf("failed to scale deploy %q.%q in cluster %q: %s", appName, componentName, clusterName, err.Error())
	}
	return nil
}

func WaitPodsStarted(ctx context.Context, clusterName, appName string) error {
	cs, err := getClientsetByClusterName(clusterName)
	if err != nil {
		return err
	}

	checker := func() (allStarted bool, err error) {
		pods, err := cs.CoreV1().Pods(appName).List(metav1.ListOptions{})
		if err != nil {
			return false, err
		}
		for _, pod := range pods.Items {
			if pod.Status.Phase != v1.PodRunning {
				return false, nil
			}
		}

		return true, nil
	}

	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("pods still not running, but timeout exeeded")
		case <-time.After(podWaitCheckInterval):
			started, err := checker()
			if err != nil {
				return err
			}
			if started {
				return nil
			}
		}
	}
}

func CreateService(clusterName, appName, serviceName, component string, port, targetPort int) error {
	cs, err := getClientsetByClusterName(clusterName)
	if err != nil {
		return err
	}

	svc := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      serviceName,
			Namespace: appName,
		},
		Spec: v1.ServiceSpec{
			Selector: map[string]string{"component": component},
			Ports: []v1.ServicePort{
				{
					Port:       int32(port),
					TargetPort: intstr.FromInt(targetPort),
				},
			},
		},
	}

	_, err = cs.CoreV1().Services(appName).Create(svc)
	if err != nil {
		return fmt.Errorf("failed to create service: %s", err.Error())
	}

	return nil
}

func UpdateServicePort(clusterName, appName, serviceName string, newPort int) error {
	cs, err := getClientsetByClusterName(clusterName)
	if err != nil {
		return err
	}

	svc, err := cs.CoreV1().Services(appName).Get(serviceName, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("failed to get service %q.%q in cluster %q: %s", appName, serviceName, clusterName, err.Error())
	}

	svc.Spec.Ports[0].Port = int32(newPort)

	_, err = cs.CoreV1().Services(appName).Update(svc)
	if err != nil {
		return fmt.Errorf("failed to update service %q.%q in cluster %q: %s", appName, serviceName, clusterName, err.Error())
	}

	return nil
}

func RemoveService(clusterName, appName, serviceName string) error {
	cs, err := getClientsetByClusterName(clusterName)
	if err != nil {
		return err
	}

	err = cs.CoreV1().Services(appName).Delete(serviceName, nil)
	if err != nil {
		return fmt.Errorf("failed to remove service: %s", err.Error())
	}

	return nil
}

func UpdateNexuses(clusterName string, deps []*v12.Nexus) error {
	cs, err := getExtClientsetByClusterName(clusterName)
	if err != nil {
		return err
	}

	err = cs.NavigatorV1().Nexuses(NavigatorNamespace).DeleteCollection(
		&metav1.DeleteOptions{GracePeriodSeconds: new(int64)},
		metav1.ListOptions{},
	)
	if err != nil {
		fmt.Printf("failed to remove old nexuses: %s\n", err.Error())
	}

	for _, dep := range deps {
		_, err := cs.NavigatorV1().Nexuses(NavigatorNamespace).Create(dep)
		if err != nil {
			return fmt.Errorf("failed to create nexus %q.%q: %s", dep.Namespace, dep.Name, err.Error())
		}
	}

	//time.Sleep(60 * time.Second)

	return nil
}

func getClusterList(clusterCount int) (clusters []string) {
	for cn := 1; cn <= clusterCount; cn++ {
		clusters = append(clusters, getClusterName(cn))
	}

	return
}

func getAppList(appCount int) (apps []string) {
	for an := 1; an <= appCount; an++ {
		apps = append(apps, getAppName(an))
	}

	return
}

func getAppName(n int) string {
	return fmt.Sprintf("%s%d", appPrefix, n)
}

func getClusterName(n int) string {
	return fmt.Sprintf("%s%d", clusterPrefix, n)
}

func CreateCanary(clusterName string, namespace, service string, backendWeights map[k8s.QualifiedName]int) (*v12.CanaryRelease, error) {
	var backends []v12.Backends
	for backendName, weight := range backendWeights {
		backend := v12.Backends{
			Name:      backendName.Name,
			Namespace: backendName.Namespace,
			Weight:    weight,
		}
		backends = append(backends, backend)
	}

	cs, err := getExtClientsetByClusterName(clusterName)
	if err != nil {
		return nil, fmt.Errorf("failed to get clientset for cluster %q: %#v", clusterName, err)
	}

	canary := &v12.CanaryRelease{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      service,
		},
		Spec: v12.CanaryReleaseSpec{
			Backends: backends,
		},
	}

	canary, err = cs.NavigatorV1().CanaryReleases(namespace).Create(canary)
	if err != nil {
		err = fmt.Errorf("failed to create canaryRelease in cluster %q: %#v", clusterName, err)
	}

	return canary, err
}

func UpdateCanary(clusterName string, namespace, service string, backendWeights map[k8s.QualifiedName]int) (*v12.CanaryRelease, error) {
	cs, err := getExtClientsetByClusterName(clusterName)
	if err != nil {
		return nil, fmt.Errorf("failed to get clientset for cluster %q: %#v", clusterName, err)
	}

	canary, err := cs.NavigatorV1().CanaryReleases(namespace).Get(service, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get canaryRelease %q.%q in cluster %q: %#v", namespace, service, clusterName, err)
	}

	canary.Spec.Backends = canary.Spec.Backends[:0]
	for backendName, weight := range backendWeights {
		backend := v12.Backends{
			Name:      backendName.Name,
			Namespace: backendName.Namespace,
			Weight:    weight,
		}
		canary.Spec.Backends = append(canary.Spec.Backends, backend)
	}

	canary, err = cs.NavigatorV1().CanaryReleases(namespace).Update(canary)
	if err != nil {
		err = fmt.Errorf("failed to update canaryRelease in cluster %q: %#v", clusterName, err)
	}
	return canary, err
}

func DeleteCanary(clusterName string, namespace, service string) error {
	cs, err := getExtClientsetByClusterName(clusterName)
	if err != nil {
		return fmt.Errorf("failed to get clientset for cluster %q: %#v", clusterName, err)
	}

	err = cs.NavigatorV1().CanaryReleases(namespace).Delete(service, nil)
	if err != nil {
		return fmt.Errorf("failed to update canaryRelease in cluster %q: %#v", clusterName, err)
	}
	return nil
}
