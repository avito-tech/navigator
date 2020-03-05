package main

import (
	"encoding/json"
	"fmt"
	"github.com/avito-tech/navigator/tests/e2e/tests"
	"net/http"
	"os"
	"strconv"
	"time"
)

func main() {
	if len(os.Args) != 3 {
		panic(fmt.Sprintf("usage: %s ADDR:PORT CLUSTER_NAME", os.Args[0]))
	}

	listenAddr := os.Args[1]
	server := &http.Server{Addr: listenAddr}
	doFail := 0

	http.HandleFunc("/reset", func(w http.ResponseWriter, r *http.Request) {
		server.Close()
		server = &http.Server{Addr: listenAddr}
	})

	http.HandleFunc("/500", func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		doFail, _ = strconv.Atoi(r.Form.Get("count"))
		_, _ = fmt.Fprint(w, getResponse())
	})

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if doFail > 0 {
			w.WriteHeader(http.StatusInternalServerError)
			doFail--
		}

		_, _ = fmt.Fprint(w, getResponse())
	})

	go func() {
		for {
			server.ListenAndServe()
			time.Sleep(time.Second)
		}
	}()

	select {}
}

func getResponse() string {
	hostname, _ := os.Hostname()
	response := tests.Response{
		Pod:     hostname,
		Cluster: os.Args[2],
	}
	content, _ := json.Marshal(&response)
	return string(content)
}
