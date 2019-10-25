package main

import (
	"encoding/json"
	"fmt"
	"github.com/avito-tech/navigator/tests/e2e/tests"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

var (
	defaultComponentList []string
	listenAddr           = ":80"
	defaultProbePort     = "8999"
	requestTimeout       = 10000 * time.Millisecond
	dnsPostfix           = "svc.cluster.local"
	defaultProbesCount   = 10
	parallelProbesCount  = 10
	defaultAppList       []string
)

func main() {
	if len(os.Args) > 1 {
		listenAddr = os.Args[1]
	}

	appListEnv := os.Getenv("APP_LIST")
	if appListEnv != "" {
		defaultAppList = strings.Split(appListEnv, " ")
	}

	componentListEnv := os.Getenv("COMPONENT_LIST")
	if componentListEnv != "" {
		defaultComponentList = strings.Split(componentListEnv, " ")
	}

	http.HandleFunc("/", handleAggregatedStats)

	//warmup
	//_, _ = getAggregatedStats(defaultProbesCount)

	err := http.ListenAndServe(listenAddr, nil)
	if err != nil {
		panic(err)
	}
}

func handleAggregatedStats(w http.ResponseWriter, r *http.Request) {
	appList := r.URL.Query()["appList"]
	if len(appList) == 0 {
		appList = defaultAppList
	}

	componentList := r.URL.Query()["componentList"]
	if len(componentList) == 0 {
		componentList = defaultComponentList
	}

	log.Printf("quaey: %v\ncomponentList:%v\n\n", r.URL.Query()["componentList"], componentList)

	count := defaultProbesCount
	if len(r.URL.Query()["count"]) > 0 {
		if tCount, _ := strconv.Atoi(r.URL.Query()["count"][0]); tCount > 0 {
			count = tCount
		}
	}

	probePort := defaultProbePort
	if len(r.URL.Query()["probePort"]) > 0 {
		probePort = r.URL.Query()["probePort"][0]
	}

	response, err := getAggregatedStats(appList, componentList, probePort, count)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(fmt.Sprintf("Failed to get stat: %s", err.Error())))
		return
	}

	w.Header().Add("Content-Type", "application/json")
	w.Write(response)
}

func getAggregatedStats(appList, componentList []string, probePort string, probesCount int) ([]byte, error) {
	var wg sync.WaitGroup

	var mu sync.Mutex
	stats := map[string]*tests.ComponentStat{}

	for _, appName := range appList {
		for _, componentName := range componentList {
			if appName == "" || componentName == "" {
				continue
			}
			wg.Add(1)
			go func(appName, componentName string, probesCount int) {

				stat := getComponentStats(appName, componentName, probePort, probesCount)

				mu.Lock()
				stats[stat.ComponentKey.String()] = stat
				mu.Unlock()

				wg.Done()
			}(appName, componentName, probesCount)
		}
	}

	wg.Wait()

	return json.Marshal(stats)
}

func getComponentStats(AppName, ComponentName, probePort string, count int) *tests.ComponentStat {
	log.Printf("getComponentStats %v %v %v \n", AppName, ComponentName, count)

	var wg sync.WaitGroup
	wg.Add(count)

	stat := &tests.ComponentStat{ComponentKey: tests.ComponentKey{AppName: AppName, ComponentName: ComponentName}, TotalProbes: count}
	semaphore := make(chan struct{}, parallelProbesCount)
	var netClient = &http.Client{
		Timeout: requestTimeout,
		Transport: &http.Transport{
			Proxy: http.ProxyFromEnvironment,
			DialContext: (&net.Dialer{
				Timeout:   30 * time.Second,
				DualStack: true,
			}).DialContext,
			IdleConnTimeout:       90 * time.Second,
			TLSHandshakeTimeout:   10 * time.Second,
			ExpectContinueTimeout: 1 * time.Second,
		},
	}

	for i := 0; i < count; i++ {
		go func() {
			defer wg.Done()
			semaphore <- struct{}{}
			defer func() { <-semaphore }()

			url := fmt.Sprintf("http://%s.%s.%s:%s", ComponentName, AppName, dnsPostfix, probePort)

			req, err := http.NewRequest("GET", url, nil)
			if err != nil {
				stat.AddFail(err)
				return
			}

			// uncomment on tcp tests
			// req.Header.Set("Connection", "close")

			start := time.Now()
			response, err := netClient.Do(req)
			if err != nil {
				stat.AddFail(err)
				return
			}

			buf, err := ioutil.ReadAll(response.Body)
			response.Body.Close()
			if err != nil {
				stat.AddFail(err)
				return
			}

			stat.AddResponse(string(buf), time.Since(start))
		}()
	}

	wg.Wait()

	return stat
}
