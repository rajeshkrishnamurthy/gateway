package main

import (
	"flag"
	"log"
	"net/http"
	"strings"
)

const (
	providerEndpoint     = "/sms/send"
	providerFailureToken = "FAIL"
)

var addr = flag.String("addr", ":9093", "HTTP listen address")

func main() {
	flag.Parse()

	mux := http.NewServeMux()
	mux.HandleFunc(providerEndpoint, handleSend)

	log.Printf("listening on %s", *addr)
	log.Fatal(http.ListenAndServe(*addr, mux))
}

func handleSend(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	query := r.URL.Query()
	version := strings.TrimSpace(query.Get("ver"))
	apiKey := strings.TrimSpace(query.Get("key"))
	encrpt := strings.TrimSpace(query.Get("encrpt"))
	destination := strings.TrimSpace(query.Get("dest"))
	senderID := strings.TrimSpace(query.Get("send"))
	message := query.Get("text")

	if version == "" || apiKey == "" || senderID == "" || strings.TrimSpace(message) == "" || destination == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	if encrpt != "0" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	if !isNumericDestination(destination) {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	if strings.Contains(message, providerFailureToken) {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func isNumericDestination(to string) bool {
	hasDigit := false
	for _, r := range to {
		switch {
		case r >= '0' && r <= '9':
			hasDigit = true
		case r == ' ' || r == '+' || r == '-' || r == '(' || r == ')':
		default:
			return false
		}
	}
	return hasDigit
}
