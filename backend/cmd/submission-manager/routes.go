package main

import (
	"net/http"

	"gateway/submissionmanager"
)

func newMux(server *apiServer, ui *managerUIServer, metrics *submissionmanager.Metrics) *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", handleHealthz)
	mux.HandleFunc("/readyz", handleReadyz)
	mux.Handle("/metrics", handleMetrics(metrics))
	mux.HandleFunc("/v1/intents", server.handleSubmit)
	mux.HandleFunc("/v1/intents/", server.handleGet)
	if ui != nil {
		mux.HandleFunc("/ui/history", ui.handleHistory)
	}
	return mux
}
