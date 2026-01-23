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

var addr = flag.String("addr", ":9092", "HTTP listen address")

func main() {
	flag.Parse()

	mux := http.NewServeMux()
	mux.HandleFunc(providerEndpoint, handleSend)

	log.Printf("listening on %s", *addr)
	log.Fatal(http.ListenAndServe(*addr, mux))
}

func handleSend(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	query := r.URL.Query()
	apiKey := strings.TrimSpace(query.Get("ApiKey"))
	serviceName := strings.TrimSpace(query.Get("ServiceName"))
	mobileNo := strings.TrimSpace(query.Get("MobileNo"))
	message := query.Get("Message")
	senderID := strings.TrimSpace(query.Get("SenderId"))

	if apiKey == "" || serviceName == "" || senderID == "" || strings.TrimSpace(message) == "" || mobileNo == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	if !isNumericDestination(mobileNo) {
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
