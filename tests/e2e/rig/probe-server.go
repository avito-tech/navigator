package main

import (
	"encoding/json"
	"fmt"
	"github.com/avito-tech/navigator/tests/e2e/tests"
	"net/http"
	"os"
)

func main() {
	if len(os.Args) != 3 {
		panic(fmt.Sprintf("usage: %s ADDR:PORT CLUSTER_NAME", os.Args[0]))
	}

	listenAddr := os.Args[1]

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		hostname, _ := os.Hostname()
		response := tests.Response{
			Pod:     hostname,
			Cluster: os.Args[2],
		}
		content, _ := json.Marshal(&response)
		_, _ = fmt.Fprint(w, string(content))
	})

	panic(http.ListenAndServe(listenAddr, nil))
}
