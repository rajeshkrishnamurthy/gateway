package main

import "net/http"

func newMux(server *apiServer) *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", handleHealthz)
	mux.HandleFunc("/readyz", handleReadyz)
	mux.Handle("/metrics", handleMetrics(server.metrics))
	mux.HandleFunc("/v1/delivery/provider-signal-webhook", server.handleDeliveryWebhookIngestion)
	mux.HandleFunc("/v1/intents/", server.handleDeliveryReadRoute)
	return mux
}
