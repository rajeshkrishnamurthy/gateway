package main

import (
	"net/http"
)

const defaultListenAddr = ":8070"

func newMux(ui *uiServer) *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("/", handleRoot)
	mux.HandleFunc("/healthz", handleHealthz)
	mux.HandleFunc("/readyz", handleReadyz)
	mux.HandleFunc("/ui", ui.handleOverview)
	mux.HandleFunc("/ui/services", ui.handleServices)
	mux.HandleFunc("/ui/services/start", ui.handleStart)
	mux.HandleFunc("/ui/services/stop", ui.handleStop)
	mux.HandleFunc("/ui/config", ui.handleConfig)
	mux.HandleFunc("/ui/config/clear", ui.handleConfigClear)
	mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir(ui.staticDir))))
	return mux
}

func handleRoot(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	http.Redirect(w, r, "/ui", http.StatusFound)
}

func handleHealthz(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

func handleReadyz(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}
