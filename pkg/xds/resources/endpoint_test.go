package resources

import (
	"fmt"
	"math/rand"
	"reflect"
	"testing"

	"github.com/avito-tech/navigator/pkg/k8s"
)

func TestEndpointCache_getCLA(t *testing.T) {

	ns1 := "ns1"
	ns2 := "ns2"
	claName1 := "cla1"
	epsName1 := "eps1"
	epsName2 := "eps2"
	epsName3 := "eps3"
	eps1 := k8s.NewQualifiedName(ns1, epsName1)
	eps2 := k8s.NewQualifiedName(ns1, epsName2)
	eps3 := k8s.NewQualifiedName(ns2, epsName1)
	eps4 := k8s.NewQualifiedName(ns2, epsName2)
	eps5 := k8s.NewQualifiedName(ns1, epsName3)
	eps6 := k8s.NewQualifiedName(ns2, epsName3)
	clusterName1 := "cluster1"
	clusterName2 := "cluster2"
	clusterName3 := "cluster3"
	clusterName4 := "cluster4"
	port := 8080
	backend1 := k8s.NewBackendSourceID(eps1, clusterName1)
	backend2 := k8s.NewBackendSourceID(eps2, clusterName1)
	backend3 := k8s.NewBackendSourceID(eps3, clusterName2)
	backend4 := k8s.NewBackendSourceID(eps4, clusterName2)

	// for bound check
	backend5 := k8s.NewBackendSourceID(eps5, clusterName3)
	backend6 := k8s.NewBackendSourceID(eps6, clusterName3)
	backend7 := k8s.NewBackendSourceID(eps1, clusterName3)
	backend8 := k8s.NewBackendSourceID(eps1, clusterName4)

	weight1 := 5
	epNum1 := 10
	wAddrSet1 := getWeightedAddrSet(weight1, epNum1, clusterName1)

	weight2 := 1
	epNum2 := 100
	wAddrSet2 := getWeightedAddrSet(weight2, epNum2, clusterName1)

	weightRand1 := 43
	epNumRand1 := 75
	wAddrSetRand1 := getWeightedAddrSet(weightRand1, epNumRand1, clusterName1)

	weightRand2 := 57
	epNumRand2 := 75
	wAddrSetRand2 := getWeightedAddrSet(weightRand2, epNumRand2, clusterName1)

	weight3 := 95
	epNum3 := 70
	wAddrSet3 := getWeightedAddrSet(weight3, epNum3, clusterName2)

	weight4 := 5
	epNum4 := 70
	wAddrSet4 := getWeightedAddrSet(weight4, epNum4, clusterName2)

	weight5 := 3
	epNum5 := 301
	wAddrSet5 := getWeightedAddrSet(weight5, epNum5, clusterName3)

	weight6 := 1
	epNum6 := 300
	wAddrSet6 := getWeightedAddrSet(weight6, epNum6, clusterName3)

	weight7 := 96
	epNum7 := 300
	wAddrSet7 := getWeightedAddrSet(weight7, epNum7, clusterName3)

	weight8 := 100
	epNum8 := 300
	wAddrSet8 := getWeightedAddrSet(weight8, epNum8, clusterName4)

	c := &EndpointCache{
		localityEnabled: true,
	}

	type args struct {
		localClusterID     string
		name               string
		targetPort         int
		backendsBySourceID map[k8s.BackendSourceID]k8s.WeightedAddressSet
	}
	tests := []struct {
		name string
		args args
		// map[wight]epCount
		wantedWeights map[int]int
		wantedSum     int
		clusterNum    int
	}{
		{
			name: "one backend",
			args: args{
				localClusterID: clusterName1,
				name:           claName1,
				targetPort:     port,
				backendsBySourceID: map[k8s.BackendSourceID]k8s.WeightedAddressSet{
					backend1: wAddrSet1,
				},
			},
			wantedWeights: map[int]int{5: 10},
			wantedSum:     50,
			clusterNum:    1,
		},
		{
			name: "two backends in one cluster",
			args: args{
				localClusterID: clusterName1,
				name:           claName1,
				targetPort:     port,
				backendsBySourceID: map[k8s.BackendSourceID]k8s.WeightedAddressSet{
					backend1: wAddrSet1,
					backend2: wAddrSet2,
				},
			},
			wantedWeights: map[int]int{
				50: 10,
				1:  100,
			},
			wantedSum:  600,
			clusterNum: 1,
		},
		{
			name: "two backends in one cluster random weights",
			args: args{
				localClusterID: clusterName1,
				name:           claName1,
				targetPort:     port,
				backendsBySourceID: map[k8s.BackendSourceID]k8s.WeightedAddressSet{
					backend1: wAddrSetRand1,
					backend2: wAddrSetRand2,
				},
			},
			wantedWeights: map[int]int{
				43: 75,
				57: 75,
			},
			wantedSum:  7500,
			clusterNum: 1,
		},
		{
			name: "four backends in two clusters",
			args: args{
				localClusterID: clusterName1,
				name:           claName1,
				targetPort:     port,
				backendsBySourceID: map[k8s.BackendSourceID]k8s.WeightedAddressSet{
					backend1: wAddrSet1,
					backend2: wAddrSet2,
					backend3: wAddrSet3,
					backend4: wAddrSet4,
				},
			},
			wantedWeights: map[int]int{
				1750: 10,
				35:   100,
				285:  70,
				15:   70,
			},
			wantedSum:  42000,
			clusterNum: 2,
		},
		{
			name: "eight backends in four clusters",
			args: args{
				localClusterID: clusterName1,
				name:           claName1,
				targetPort:     port,
				backendsBySourceID: map[k8s.BackendSourceID]k8s.WeightedAddressSet{
					backend1: wAddrSet1,
					backend2: wAddrSet2,
					backend3: wAddrSet3,
					backend4: wAddrSet4,
					backend5: wAddrSet5,
					backend6: wAddrSet6,
					backend7: wAddrSet7,
					backend8: wAddrSet8,
				},
			},
			wantedWeights: map[int]int{
				752500: 10,
				15050:  100,
				122550: 70,
				6450:   70,
				900:    301,
				301:    300,
				28896:  300,
				30100:  300,
			},
			wantedSum:  36120000,
			clusterNum: 4,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := c.getCLA(tt.args.localClusterID, tt.args.name, tt.args.targetPort, tt.args.backendsBySourceID)
			gotWeights := map[int]int{}
			gotSum := 0
			localSum := 0
			for _, eps := range got.Endpoints {
				for _, ep := range eps.LbEndpoints {
					w := int(ep.LoadBalancingWeight.Value)
					if _, ok := gotWeights[w]; !ok {
						gotWeights[w] = 0
					}
					gotWeights[w]++
					gotSum += w
					if eps.Priority == 0 {
						localSum += w
					}
				}
			}
			if !reflect.DeepEqual(gotWeights, tt.wantedWeights) {
				t.Errorf("error ep weights: got %v, want %v", gotWeights, tt.wantedWeights)
			}
			if gotSum != tt.wantedSum {
				t.Errorf("error ep weights sum: got %d, want %d", gotSum, tt.wantedSum)
			}
			if localSum == 0 {
				t.Error("test error, local cluster not found")
			}
			clustersCheck := gotSum / localSum
			wholeCheck := gotSum % localSum
			if clustersCheck != tt.clusterNum || wholeCheck != 0 {
				t.Errorf("error per cluster sum: local cluster sum %d, all clusters sum %d", localSum, gotSum)
			}
		})
	}
}

func getWeightedAddrSet(weight, addrNum int, clisterID string) k8s.WeightedAddressSet {
	addrSet := k8s.AddressSet{}
	for i := 0; i < addrNum; i++ {
		ip := fmt.Sprintf("%d.%d.%d.%d", rand.Intn(256), rand.Intn(256), rand.Intn(256), rand.Intn(256))
		addrSet[ip] = k8s.Address{
			IP:        ip,
			ClusterID: clisterID,
		}
	}
	return k8s.WeightedAddressSet{
		AddrSet: addrSet,
		Weight:  weight,
	}
}
