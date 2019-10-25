//+build e2e_test

package tests

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"testing"
	"time"

	"github.com/avito-tech/navigator/pkg/k8s"

	v12 "github.com/avito-tech/navigator/pkg/apis/navigator/v1"
)

const (
	clusterCount           = 2
	appCount               = 3
	defaultAppStatsTimeout = 30 * time.Second
	defaultDeployTimeout   = 3 * time.Minute
	statsProbesCount       = 50
	defaultProbePort       = 8999
	defaultReplicasCount   = 2
	normalizedWeight       = 100
	wantBiasRatio          = 0.3
)

const (
	unstableConnectionsRetries = 5
)

var (
	defaultComponentList = []string{"main", "aux"}
	defaultDiscovery     = Discovery{
		"test-1": map[string]map[string]bool{
			"test-1": {WholeNamespace: true},
			"test-2": {WholeNamespace: true},
			"test-3": {WholeNamespace: true},
		},
		"test-2": map[string]map[string]bool{
			"test-1": {WholeNamespace: true},
			"test-2": {WholeNamespace: true},
		},
		"test-3": map[string]map[string]bool{
			"test-3": {WholeNamespace: true},
		},
	}
)

func TestBaseDeploy(t *testing.T) {
	discovery := SetupTest(t, appCount)
	if err := checkCoherence(t, discovery, appCount); err != nil {
		t.Error(err.Error())
	}
}

func TestScale(t *testing.T) {
	discovery := SetupTest(t, appCount)
	cases := []struct {
		srcClusterName, dstClusterName, appName, componentName string
		replicas                                               int32
	}{
		{
			srcClusterName: getClusterName(1),
			dstClusterName: getClusterName(1),
			appName:        getAppName(1),
			componentName:  defaultComponentList[0],
			replicas:       defaultReplicasCount + 1,
		},
		{
			srcClusterName: getClusterName(1),
			dstClusterName: getClusterName(2),
			appName:        getAppName(1),
			componentName:  defaultComponentList[0],
			replicas:       defaultReplicasCount + 2,
		},
		{
			srcClusterName: getClusterName(1),
			dstClusterName: getClusterName(1),
			appName:        getAppName(1),
			componentName:  defaultComponentList[0],
			replicas:       defaultReplicasCount - 1,
		},
		{
			srcClusterName: getClusterName(1),
			dstClusterName: getClusterName(2),
			appName:        getAppName(1),
			componentName:  defaultComponentList[0],
			replicas:       defaultReplicasCount,
		},
	}

	for i, c := range cases {
		err := ScaleReplicaset(defaultDeployTimeout, c.dstClusterName, c.appName, c.componentName, c.replicas)
		if err != nil {
			t.Fatal(err.Error())
		}

		//fmt.Printf("Sleeping\n")
		time.Sleep(5 * time.Second) //wait for envoy to sync
		//fmt.Printf("Wake!\n")

		if err := checkCoherence(t, discovery, appCount); err != nil {
			t.Errorf("Failed case #%d: %s", i, err.Error())
		}
	}
}

func TestAddService(t *testing.T) {
	cases := []struct {
		srcClusterName, dstClusterName, appName, dstComponentName string
		port, targetPort                                          int
		wantAccessible                                            bool
	}{
		{
			srcClusterName:   getClusterName(1),
			dstClusterName:   getClusterName(1),
			appName:          getAppName(1),
			dstComponentName: "new",
			port:             7999,
			targetPort:       80,
			wantAccessible:   true,
		},
		{
			srcClusterName:   getClusterName(1),
			dstClusterName:   getClusterName(2),
			appName:          getAppName(1),
			dstComponentName: "new",
			port:             7999,
			targetPort:       80,
			wantAccessible:   false,
		},
	}

	for i, c := range cases {
		discovery := SetupTest(t, appCount)

		err := CreateService(c.dstClusterName, c.appName, c.dstComponentName, c.dstComponentName, c.port, c.targetPort)
		if err != nil {
			t.Fatal(err.Error())
		}

		// we mark new service as "want accessible" / "want inaccessible" using discovery
		// to check that if service exists ONLY in the same cluster with src App, it SHOULD be accessible,
		// but if service exists ONLY in the DIFFERENT cluster than src App, it should NOT be accessible
		discovery.Add(c.appName, c.appName, c.dstComponentName, c.wantAccessible)

		if err := checkCoherence(t, discovery, appCount); err != nil {
			t.Errorf("Failed case #%d check full coherence: %s", i, err.Error())
		}

		err = CheckAppStats(
			t,
			c.srcClusterName,
			c.appName,
			discovery,
			[]string{c.appName},
			[]string{c.dstComponentName},
			[]string{c.dstClusterName},
			nil,
			c.port,
			statsProbesCount,
			false,
		)

		if err != nil {
			t.Errorf("Failed case #%d  check new component: %s", i, err.Error())
		}
	}
}

func TestRemoveService(t *testing.T) {
	cases := []struct {
		srcClusterName, dstClusterName, appName, componentName string
		wantAccessible                                         map[string]bool
	}{
		{
			srcClusterName: getClusterName(1),
			dstClusterName: getClusterName(1),
			appName:        getAppName(1),
			componentName:  defaultComponentList[0],
			wantAccessible: map[string]bool{getClusterName(1): false, getClusterName(2): false},
		},
		{
			srcClusterName: getClusterName(1),
			dstClusterName: getClusterName(2),
			appName:        getAppName(1),
			componentName:  defaultComponentList[0],
			wantAccessible: map[string]bool{getClusterName(1): true, getClusterName(2): false},
		},
	}

	for i, c := range cases {
		discovery := SetupTest(t, appCount)

		err := RemoveService(c.dstClusterName, c.appName, c.componentName)
		if err != nil {
			t.Fatal(err.Error())
		}

		// This test checks 2 test cases:
		// 1. Deleting svc entity from THE SAME cluster with request origins.
		// E.g. request from cluster `cluster-1`, delete svc from cluster `cluster-1`
		// 2. Deleting svc entity from REMOTE cluster in relation to request origins.
		// E.g. request from cluster `cluster-1`, delete svc from cluster `cluster-2`
		// Thus, we should not to test FULL coherence from every cluster to every cluster
		// We should check accessibility from cluster-1 to cluster-1 and from cluster-1 to cluster-1.
		// checkFullCoherence() performs also `from cluster-2 to cluster-2` and `from cluster-2 to cluster-2`
		// but we couuld skip it to cimplify test
		// due to this additional check are symmetric to first two checks
		for dstClusterName, wantAccessible := range c.wantAccessible {
			// we mark DELETING service as "want accessible" / "want inaccessible" using discovery
			// to check that if the service deleted from the same cluster with src App, it should NOT be accessible,
			// but if service deleted from the DIFFERENT cluster than src App, it SHOULD be accessible
			discovery.DeclareExistingDestinationAccessibility(c.appName, c.componentName, wantAccessible)

			for _, appName := range getAppList(appCount) {
				err = CheckAppStats(
					t,
					c.srcClusterName,
					appName,
					discovery,
					getAppList(appCount),
					defaultComponentList,
					[]string{dstClusterName},
					nil,
					defaultProbePort,
					statsProbesCount,
					false,
				)
				if err != nil {
					t.Fatalf("Failed case #%d check stats: %s", i, err.Error())
				}
			}
		}
	}
}

func TestUpdateService(t *testing.T) {
	cases := []struct {
		appName, componentName string
		newPort                int
	}{
		{appName: getAppName(1), componentName: defaultComponentList[0], newPort: 7999},
	}

	for i, c := range cases {
		discovery := SetupTest(t, appCount)

		for _, clusterName := range getClusterList(clusterCount) {
			err := UpdateServicePort(clusterName, c.appName, c.componentName, c.newPort)
			time.Sleep(5 * time.Second) //wait for envoy to sync
			if err != nil {
				t.Fatal(err.Error())
			}
		}

		for _, clusterName := range getClusterList(clusterCount) {
			// mark updated component NOT accessible via old port (defaultProbePort)
			// while all other services should be still accessible according to its discovery via default port
			discovery.DeclareExistingDestinationAccessibility(c.appName, c.componentName, false)

			if err := checkCoherence(t, discovery, appCount); err != nil {
				t.Errorf("Failed case #%d check full coherence: %s", i, err.Error())
			}

			//  using discovery object, mark updated service accessible via NEW port
			discovery.DeclareExistingDestinationAccessibility(c.appName, c.componentName, true)
			err := CheckAppStats(
				t,
				clusterName,
				c.appName,
				discovery,
				[]string{c.appName},
				[]string{c.componentName},
				getClusterList(clusterCount),
				nil,
				c.newPort,
				statsProbesCount,
				false,
			)

			if err != nil {
				t.Errorf("Failed case #%d check updated components: %s", i, err.Error())
			}
		}
	}
}
func TestAddNexus(t *testing.T) {
	newAppCount := appCount + 2

	cases := [][]struct {
		srcAppName, dstAppName string
	}{
		{
			{srcAppName: getAppName(appCount + 1), dstAppName: getAppName(1)},
			{srcAppName: getAppName(appCount + 1), dstAppName: getAppName(2)},
			{srcAppName: getAppName(appCount + 2), dstAppName: getAppName(1)},
			{srcAppName: getAppName(appCount + 10), dstAppName: getAppName(100)},
		},
	}

	for i, c := range cases {
		referenceDiscovery := SetupTest(t, newAppCount)
		discoveries := []Discovery{referenceDiscovery.Copy()}
		for _, rule := range c {
			discovery := NewDiscovery()
			discoveries = append(discoveries, discovery)

			// we use one discovery per rule to test discovery merge
			discovery.Add(rule.srcAppName, rule.dstAppName, WholeNamespace, true)

			// and use separate referenceDiscovery to convey it to checkCoherence
			referenceDiscovery.Add(rule.srcAppName, rule.dstAppName, WholeNamespace, true)
		}

		err := UpdateDiscovery(clusterCount, discoveries)
		if err != nil {
			t.Fatalf("Failed to update discovery in case #%d: %s", i, err.Error())
		}

		if err := checkCoherence(t, referenceDiscovery, newAppCount); err != nil {
			t.Errorf("Failed case #%d check full coherence: %s", i, err.Error())
		}
	}
}

func TestUpdateNexus(t *testing.T) {
	cases := [][]struct {
		srcAppName  string
		dstAppNames []string
	}{
		{
			{
				srcAppName:  getAppName(1),
				dstAppNames: []string{getAppName(1)},
			},
			{
				srcAppName:  getAppName(2),
				dstAppNames: []string{getAppName(1), getAppName(2), getAppName(3)},
			},
			{
				srcAppName:  getAppName(3),
				dstAppNames: []string{getAppName(2)},
			},
		},
		{
			{
				srcAppName:  getAppName(1),
				dstAppNames: nil,
			},
		},
	}

	for i, c := range cases {
		discovery := SetupTest(t, appCount)

		for _, rule := range c {
			discovery.FlushDestinations(rule.srcAppName)
			for _, dstAppName := range rule.dstAppNames {
				discovery.Add(rule.srcAppName, dstAppName, WholeNamespace, true)
			}
		}

		err := UpdateDiscovery(clusterCount, []Discovery{discovery})
		if err != nil {
			t.Fatalf("Failed to update discovery in case #%d: %s", i, err.Error())
		}

		if err := checkCoherence(t, discovery, appCount); err != nil {
			t.Errorf("Failed case #%d check full coherence: %s", i, err.Error())
		}
	}
}

func TestDeleteNexus(t *testing.T) {
	cases := [][]struct {
		srcAppName string
	}{
		{
			{srcAppName: getAppName(1)},
		},
		{
			{srcAppName: getAppName(1)},
			{srcAppName: getAppName(2)},
		},
	}

	for i, c := range cases {
		discovery := SetupTest(t, appCount)

		for _, rule := range c {
			discovery.RemoveSource(rule.srcAppName)
		}

		err := UpdateDiscovery(clusterCount, []Discovery{discovery})
		if err != nil {
			t.Fatalf("Failed to update discovery in case #%d: %s", i, err.Error())
		}

		if err := checkCoherence(t, discovery, appCount); err != nil {
			t.Errorf("Failed case #%d check full coherence: %s", i, err.Error())
		}
	}
}

type canaryTestConfig struct {
	clusterNums          []int
	appNum, componentNum int
	backendAppByWeights  map[int]int
	update, del          bool
}

func TestCreateCanary(t *testing.T) {
	cases := [][]canaryTestConfig{
		0: { // different canaries for different services symmetrical in both clusters
			{
				clusterNums:         []int{1, 2},
				appNum:              1,
				componentNum:        0,
				backendAppByWeights: map[int]int{1: 75, 2: 25},
			},
			{
				clusterNums:         []int{1, 2},
				appNum:              3,
				componentNum:        0,
				backendAppByWeights: map[int]int{1: 75, 2: 25},
			},
		},
		1: { // 1. different canaries for different services in one cluster
			{
				clusterNums:         []int{1},
				appNum:              1,
				componentNum:        0,
				backendAppByWeights: map[int]int{1: 75, 2: 25},
			},
			{
				clusterNums:         []int{1},
				appNum:              3,
				componentNum:        0,
				backendAppByWeights: map[int]int{3: 100},
			},
		},
		2: { // canary to other services
			{
				clusterNums:         []int{1},
				appNum:              2,
				componentNum:        0,
				backendAppByWeights: map[int]int{1: 75, 3: 25},
			},
		},
		3: { // canary from already canaried service to both other services
			{
				clusterNums:         []int{1},
				appNum:              1,
				componentNum:        0,
				backendAppByWeights: map[int]int{1: 75, 2: 25},
			},
			{
				clusterNums:         []int{1},
				appNum:              2,
				componentNum:        0,
				backendAppByWeights: map[int]int{1: 75, 3: 25},
			},
			{
				clusterNums:         []int{1},
				appNum:              3,
				componentNum:        0,
				backendAppByWeights: map[int]int{3: 75, 2: 25},
			},
		},
		4: { // canary in one cluster, check after endpoints changed
			{
				clusterNums:         []int{1},
				appNum:              1,
				componentNum:        0,
				backendAppByWeights: map[int]int{1: 75, 2: 25},
			},
			{
				clusterNums:         []int{1},
				appNum:              3,
				componentNum:        0,
				backendAppByWeights: map[int]int{2: 75, 3: 25},
			},
		},
		5: { // different canaries for different services in different clusters
			{
				clusterNums:         []int{1},
				appNum:              1,
				componentNum:        0,
				backendAppByWeights: map[int]int{1: 75, 2: 25},
			},
			{
				clusterNums:         []int{2},
				appNum:              3,
				componentNum:        0,
				backendAppByWeights: map[int]int{1: 75, 2: 25},
			},
		},
		6: { // different canaries for the SAME service in different clusters
			{
				clusterNums:         []int{1},
				appNum:              1,
				componentNum:        0,
				backendAppByWeights: map[int]int{1: 75, 2: 25},
			},
			{
				clusterNums:         []int{2},
				appNum:              1,
				componentNum:        0,
				backendAppByWeights: map[int]int{2: 75, 3: 25},
			},
		},
	}

	for i, configs := range cases {
		discovery := SetupTest(t, appCount)
		if err := checkCanary(t, configs, discovery); err != nil {
			t.Errorf("Failed case #%d: %s", i, err.Error())
		}
	}
}

func TestUpdateCanary(t *testing.T) {
	cases := [][]canaryTestConfig{
		0: { //switch existing canary to other version of "new" release
			{ // first, create canary test-1 -> {test-1, test-2}
				clusterNums:         []int{1},
				appNum:              1,
				componentNum:        0,
				backendAppByWeights: map[int]int{1: 75, 2: 25},
			},
			{ // then, update canary test-1 -> {test-1, test-3}
				clusterNums:         []int{1},
				appNum:              1,
				componentNum:        0,
				backendAppByWeights: map[int]int{1: 75, 3: 25},
				update:              true,
			},
		},
		1: { //switch existing canary from 75/25 -> 50/50 old/new
			{ // first, create canary test-1 -> {test-1@75, test-2@25}
				clusterNums:         []int{1},
				appNum:              1,
				componentNum:        0,
				backendAppByWeights: map[int]int{1: 75, 2: 25},
			},
			{ // then, update canary test-1 -> {test-1@50, test-2@50}
				clusterNums:         []int{1},
				appNum:              1,
				componentNum:        0,
				backendAppByWeights: map[int]int{1: 50, 2: 50},
				update:              true,
			},
		},
		2: { //switch existing canary to 100% "new" release by removing "old" backends from canary
			{ // first, create canary test-1 -> {test-1@75, test-2@25}
				clusterNums:         []int{1},
				appNum:              1,
				componentNum:        0,
				backendAppByWeights: map[int]int{1: 75, 2: 25},
			},
			{ // then, update canary test-1 -> {test-2@100}
				clusterNums:         []int{1},
				appNum:              1,
				componentNum:        0,
				backendAppByWeights: map[int]int{2: 100},
				update:              true,
			},
		},
	}

	for i, configs := range cases {
		discovery := SetupTest(t, appCount)
		if err := checkCanary(t, configs, discovery); err != nil {
			t.Errorf("Failed case #%d: %s", i, err.Error())
		}
	}
}

func TestDeleteCanary(t *testing.T) {
	cases := [][]canaryTestConfig{
		0: { //switch existing canary to 100% "old" release by removing canary in both clusters
			{ // first, create canary test-1 -> {test-1@75, test-2@25}
				clusterNums:         []int{1, 2},
				appNum:              1,
				componentNum:        0,
				backendAppByWeights: map[int]int{1: 75, 2: 25},
			},
			{ // then, delete canaries in both clusters
				clusterNums:  []int{1, 2},
				appNum:       1,
				componentNum: 0,
				del:          true,
			},
		},
		1: { //switch existing canary to 100% "old" release by removing canary in one cluster
			{ // first, create canary test-1 -> {test-1@75, test-2@25}
				clusterNums:         []int{1},
				appNum:              1,
				componentNum:        0,
				backendAppByWeights: map[int]int{1: 75, 2: 25},
			},
			{ // then, delete canaries in one clusters
				clusterNums:  []int{1},
				appNum:       1,
				componentNum: 0,
				del:          true,
			},
		},
		2: { //switch existing canary to 100% "old" release by removing canary only in one cluster
			{ // first, create canary test-1 -> {test-1@75, test-2@25}
				clusterNums:         []int{1, 2},
				appNum:              1,
				componentNum:        0,
				backendAppByWeights: map[int]int{1: 75, 2: 25},
			},
			{ // then, delete canaries in 2nd cluster
				clusterNums:  []int{2},
				appNum:       1,
				componentNum: 0,
				del:          true,
			},
		},
	}

	for i, configs := range cases {
		discovery := SetupTest(t, appCount)
		if err := checkCanary(t, configs, discovery); err != nil {
			t.Errorf("Failed case #%d: %s", i, err.Error())
		}
	}
}

func checkCanary(t *testing.T, configs []canaryTestConfig, discovery Discovery) error {
	canariesByCluster := map[string]map[k8s.QualifiedName]*v12.CanaryRelease{}
	for _, c := range configs {
		appName := getAppName(c.appNum)
		componentName := defaultComponentList[c.componentNum]
		backendWeights := map[k8s.QualifiedName]int{}
		for canaryAppNum, weight := range c.backendAppByWeights {
			backendWeights[k8s.NewQualifiedName(getAppName(canaryAppNum), componentName)] = weight
		}

		for _, clusterNum := range c.clusterNums {
			clusterName := getClusterName(clusterNum)
			if canariesByCluster[clusterName] == nil {
				canariesByCluster[clusterName] = map[k8s.QualifiedName]*v12.CanaryRelease{}
			}

			var (
				canary *v12.CanaryRelease
				err    error
			)

			if c.del {
				err = DeleteCanary(clusterName, appName, componentName)
				delete(canariesByCluster[clusterName], k8s.NewQualifiedName(appName, componentName))
				if err != nil {
					return fmt.Errorf("failed to del canaryRelease: %s", err.Error())
				}
			} else if c.update {
				canary, err = UpdateCanary(clusterName, appName, componentName, backendWeights)
				canariesByCluster[clusterName][k8s.NewQualifiedName(appName, componentName)] = canary
				if err != nil {
					return fmt.Errorf("failed to update canaryRelease: %s", err.Error())
				}
			} else {
				canary, err = CreateCanary(clusterName, appName, componentName, backendWeights)
				canariesByCluster[clusterName][k8s.NewQualifiedName(appName, componentName)] = canary
				if err != nil {
					return fmt.Errorf("failed to create canaryRelease: %s", err.Error())
				}
			}
		}

		time.Sleep(2 * time.Second) //ensure navigator processed update
	}

	err := checkCoherenceCanary(t, discovery, canariesByCluster, 1000)
	if err != nil {
		return err
	}

	for _, c := range configs {
		componentName := defaultComponentList[c.componentNum]
		for _, clusterNum := range c.clusterNums {
			for _, canaryAppName := range getAppList(appCount) {
				err = ScaleReplicaset(defaultDeployTimeout, getClusterName(clusterNum), canaryAppName, componentName, defaultReplicasCount-1)
				if err != nil {
					panic(err.Error())
				}
			}
		}
	}

	time.Sleep(3 * time.Second) //ensure navigator processed update

	return checkCoherenceCanary(t, discovery, canariesByCluster, 1000)
}

func SetupTest(t *testing.T, appCount int) Discovery {
	err := Clean(defaultDeployTimeout, clusterCount, getAppList(appCount))
	if err != nil {
		t.Fatal(err.Error())
	}

	err = Setup(defaultDeployTimeout, clusterCount, getAppList(appCount))
	if err != nil {
		t.Fatalf("%s", err.Error())
	}

	// deep copy discovery
	discovery := defaultDiscovery.Copy()

	err = UpdateDiscovery(clusterCount, []Discovery{discovery})
	if err != nil {
		t.Fatalf("%s", err.Error())
	}

	time.Sleep(1 * time.Second) // wait discovery updated

	return discovery
}

func CheckAppStats(
	t *testing.T,
	srcClusterName,
	appName string,
	discovery Discovery,
	appList,
	componentList,
	dstClusterNames []string,
	canariesByCluster map[string]map[k8s.QualifiedName]*v12.CanaryRelease,
	port int,
	probesCount int,
	checkWeights bool,
) error {
	ctx := context.WithValue(context.Background(), unstableConnectionsRetries, unstableConnectionsRetries)
	stats, err := doCheckAppStats(
		ctx,
		srcClusterName,
		appName,
		discovery,
		appList,
		componentList,
		dstClusterNames,
		canariesByCluster,
		port,
		probesCount,
		checkWeights,
	)
	if err != nil {
		t.Log(getPrettyStats(srcClusterName, appName, stats))
		return fmt.Errorf("cluster=%q srcApp=%q: %s", srcClusterName, appName, err)
	}

	logs, err := getLogs(ctx, srcClusterName, NavigatorNamespace, "navigator")
	if err != nil {
		return err
	}

	warns := getDataRaceWarnings(logs)
	if len(warns) > 0 {
		t.Fatalf("Data race detected!\n\n%v", warns)
	}

	return nil
}

func doCheckAppStats(
	ctx context.Context,
	srcClusterName,
	appName string,
	discovery Discovery,
	appList,
	componentList,
	dstClusterNames []string,
	canariesByCluster map[string]map[k8s.QualifiedName]*v12.CanaryRelease,
	port int,
	probesCount int,
	checkWeights bool,
) (stat ComponentStat, err error) {
	getStatsCtx, _ := context.WithDeadline(ctx, time.Now().Add(defaultAppStatsTimeout))
	stats, err := getAppStats(getStatsCtx, srcClusterName, appName, appList, componentList, port, probesCount)
	if err != nil {
		return ComponentStat{}, fmt.Errorf("failed to fetch app stats: %s", err.Error())
	}

	type culsterName = string
	type serviceName = k8s.QualifiedName
	type endpointsName = k8s.QualifiedName
	componentWeightsByServiceByCluster := map[culsterName]map[serviceName]map[endpointsName]int{}
	for clusterName, canaries := range canariesByCluster {
		if componentWeightsByServiceByCluster[clusterName] == nil {
			componentWeightsByServiceByCluster[clusterName] = map[serviceName]map[endpointsName]int{}
		}

		for qn, canary := range canaries {
			componentWeightsByServiceByCluster[clusterName][qn] = map[endpointsName]int{}
			for _, backend := range canary.Spec.Backends {
				componentWeightsByServiceByCluster[clusterName][qn][k8s.NewQualifiedName(backend.Namespace, backend.Name)] = backend.Weight
			}
		}
	}

	for _, dstAppName := range appList {
		for _, dstComponentName := range componentList {
			key := ComponentKey{AppName: dstAppName, ComponentName: dstComponentName}.String()

			stat, ok := stats[key]
			if !ok {
				return ComponentStat{}, fmt.Errorf(
					"there is no stat for destination \"%s.%s\" in app stats",
					dstAppName, dstComponentName,
				)
			}

			if stat.IsInvalid() {
				return stat, fmt.Errorf(
					"stat for destination \"%s.%s\" is has invalid responses",
					dstAppName, dstComponentName,
				)
			}

			if stat.IsUnstable() {
				time.Sleep(5 * time.Second) // wait for network stabilizes
				count, _ := ctx.Value(unstableConnectionsRetries).(int)
				if count <= 0 {
					return stat, fmt.Errorf(
						"stats to target %q.%q stays unstable and no more getAppStats retries left",
						dstAppName,
						dstComponentName,
					)
				}
				nextCtx := context.WithValue(ctx, unstableConnectionsRetries, count-1)
				return doCheckAppStats(
					nextCtx,
					srcClusterName,
					appName,
					discovery,
					appList,
					componentList,
					dstClusterNames,
					canariesByCluster,
					port,
					probesCount,
					checkWeights,
				)
			}

			wantAccessible := discovery.IsAccessible(appName, stat.AppName, stat.ComponentName)
			WantPodsWeightsByCluster, err := getWantPodsWeightsByCluster(
				dstClusterNames,
				dstAppName,
				dstComponentName,
				componentWeightsByServiceByCluster,
				normalizedWeight,
			)

			if err != nil {
				return stat, fmt.Errorf("failed to get want pod names: %s", err)
			}

			gotAccessible := stat.IsAccessible(WantPodsWeightsByCluster)

			if wantAccessible != gotAccessible {
				return stat, fmt.Errorf(
					"stat for destination \"%s.%s\" has accessibility %t when want %t. \nCurrent discovery: \n\n====\n%v\n====",
					dstAppName, dstComponentName,
					gotAccessible,
					wantAccessible,
					discovery,
				)
			}

			if !wantAccessible {
				continue
			}

			gotAllBackendsAccessible := stat.IsAllBackendsAccessible(WantPodsWeightsByCluster)
			if !gotAllBackendsAccessible {
				return stat, getBalancingError(dstAppName, dstComponentName, WantPodsWeightsByCluster, stat)
			}

			if checkWeights {
				err = stat.CheckBalancingWeighted(WantPodsWeightsByCluster, wantBiasRatio)
				if err != nil {
					return stat, err
				}
			}
		}
	}

	return stat, nil
}

func getWantPodsWeightsByCluster(
	dstClusterNames []string,
	dstAppName, dstComponentName string,
	componentWeightsByServiceByCluster map[string]map[k8s.QualifiedName]map[k8s.QualifiedName]int,
	normalizedWeight int,
) (map[string]map[string]int, error) {
	wantPodNames := map[string]map[string]int{}

	for _, dstClusterName := range dstClusterNames {
		backendSetWeights := componentWeightsByServiceByCluster[dstClusterName][k8s.NewQualifiedName(dstAppName, dstComponentName)]
		if backendSetWeights == nil {
			backendSetWeights = map[k8s.QualifiedName]int{{Namespace: dstAppName, Name: dstComponentName}: normalizedWeight}
		}

		totalWeight := 0
		for _, weight := range backendSetWeights {
			totalWeight += weight
		}
		normalizingRatio := float64(normalizedWeight) / float64(totalWeight)

		for qn, weight := range backendSetWeights {
			podNames, err := GetPodNames(dstClusterName, qn.Namespace, qn.Name)
			if err != nil {
				return nil, err
			}

			if wantPodNames[dstClusterName] == nil {
				wantPodNames[dstClusterName] = map[string]int{}
			}

			for _, podName := range podNames {
				wantPodNames[dstClusterName][podName] = int(float64(weight) * normalizingRatio / float64(len(podNames)))
			}
		}
	}

	return wantPodNames, nil
}

func getBalancingError(dstAppName, dstComponentName string, wantPodWeightsByCluster map[string]map[string]int, stat ComponentStat) error {
	gotPodsByCluster := map[string][]string{}
	for clusterName, responses := range stat.ResponsesByCluster {
		for pod := range responses {
			gotPodsByCluster[clusterName] = append(gotPodsByCluster[clusterName], pod)
		}
		sort.Strings(gotPodsByCluster[clusterName])
	}

	prettyGotPods, _ := json.MarshalIndent(gotPodsByCluster, "", "  ")
	prettyWantPods, _ := json.MarshalIndent(wantPodWeightsByCluster, "", "  ")

	return fmt.Errorf(
		"dectination \"%s.%s\" \n want balanced to pods:\n %s\n\n got pods:\n %s",
		dstAppName, dstComponentName,
		prettyWantPods,
		prettyGotPods,
	)
}

func getPrettyStats(clusterName, appName string, stat ComponentStat) string {
	prettyStats, _ := json.MarshalIndent(stat, "", "  ")
	return fmt.Sprintf("App Stats for cluster %q app %q:\n\n%s\n\n", clusterName, appName, prettyStats)
}

func checkCoherence(t *testing.T, discovery Discovery, appCount int) error {
	for _, clusterName := range getClusterList(clusterCount) {
		for _, appName := range getAppList(appCount) {
			if err := CheckAppStats(
				t,
				clusterName,
				appName,
				discovery,
				getAppList(appCount),
				defaultComponentList,
				getClusterList(clusterCount),
				nil,
				defaultProbePort,
				statsProbesCount,
				false,
			); err != nil {
				return err
			}
		}
	}

	return nil
}

func checkCoherenceCanary(t *testing.T, discovery Discovery, canariesByCluster map[string]map[k8s.QualifiedName]*v12.CanaryRelease, probesCount int) error {
	for _, clusterName := range getClusterList(clusterCount) {
		for _, appName := range getAppList(appCount) {
			if err := CheckAppStats(
				t,
				clusterName,
				appName,
				discovery,
				getAppList(appCount),
				defaultComponentList,
				getClusterList(clusterCount),
				canariesByCluster,
				defaultProbePort,
				probesCount,
				true,
			); err != nil {
				return err
			}
		}
	}

	return nil
}

func getDataRaceWarnings(logs string) (warnings []string) {
	re := regexp.MustCompile("(?msU)==================\nWARNING: DATA RACE\n.+\n==================")
	return re.FindAllString(logs, -1)
}
