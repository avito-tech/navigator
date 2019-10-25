package handlers

import (
	"net/http"
)

func RegisterHealthCheck(mux *http.ServeMux) {
	mux.HandleFunc("/health", health)
}

func health(res http.ResponseWriter, _ *http.Request) {
	_, _ = res.Write([]byte("OK"))
}
