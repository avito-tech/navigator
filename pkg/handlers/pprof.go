package handlers

import (
	"fmt"
	"net/http"
	"net/http/pprof"
	"runtime"
	"strconv"
)

func RegisterProfile(mux *http.ServeMux) {
	mux.HandleFunc("/debug/pprof/", pprof.Index)
	mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
	mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	mux.HandleFunc("/debug/pprof/trace", pprof.Trace)

	mux.HandleFunc("/debug/blockrate", blockRate)
	mux.HandleFunc("/debug/pprof/block", blockProfile)

	mux.HandleFunc("/debug/mutexrate", mutexRate)
	mux.HandleFunc("/debug/pprof/mutex", mutexProfile)
}

func blockRate(res http.ResponseWriter, req *http.Request) {
	// this endpoint calls runtime.SetBlockProfileRate(v), where v is an optional
	// query string parameter defaulting to 10000 (1 sample per 10μs blocked)

	if req.Method != http.MethodPost {
		http.Error(res, fmt.Sprintf("expected POST method got %s", req.Method), http.StatusMethodNotAllowed)
		return
	}

	rate := 10000
	v := req.URL.Query().Get("v")
	if v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			http.Error(res, "v must be an integer", http.StatusBadRequest)
			return
		}
		rate = n
	}
	runtime.SetBlockProfileRate(rate)
	_, _ = res.Write([]byte(fmt.Sprintf("Block profile rate set to %d. It will automatically be disabled again after calling block profile handler\n", rate)))
}

func blockProfile(res http.ResponseWriter, req *http.Request) {
	// serve the block profile and reset the rate to 0
	pprof.Handler("block").ServeHTTP(res, req)
	runtime.SetBlockProfileRate(0)
}

func mutexRate(res http.ResponseWriter, req *http.Request) {
	// this endpoint calls runtime.SetMutexProfileFraction(v), where v is an optional
	// query string parameter defaulting to 5 (1/5 of all samples)

	if req.Method != http.MethodPost {
		http.Error(res, fmt.Sprintf("expected POST method got %s", req.Method), http.StatusMethodNotAllowed)
		return
	}

	rate := 5
	v := req.URL.Query().Get("v")
	if v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			http.Error(res, "v must be an integer", http.StatusBadRequest)
			return
		}
		rate = n
	}
	runtime.SetMutexProfileFraction(rate)
	_, _ = res.Write([]byte(fmt.Sprintf("Mutex profile rate set to %d. It will automatically be disabled again after calling mutex profile handler\n", rate)))
}

func mutexProfile(res http.ResponseWriter, req *http.Request) {
	// serve the block profile and reset the rate to 0.
	pprof.Handler("mutex").ServeHTTP(res, req)
	runtime.SetMutexProfileFraction(0)
}
