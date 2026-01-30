package main

import "net/http"

func newMux(server *apiServer) *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/intents", server.handleSubmit)
	mux.HandleFunc("/v1/intents/", server.handleGet)
	return mux
}
