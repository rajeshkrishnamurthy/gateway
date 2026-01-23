package main

import (
	"encoding/json"
	"flag"
	"io"
	"log"
	"net/http"
	"strings"
)

const (
	maxBodyBytes         = 16 << 10
	providerEndpoint     = "/sms/send"
	providerFailureToken = "FAIL"
)

var addr = flag.String("addr", ":9094", "HTTP listen address")

type infoBipRequestBody struct {
	Messages []infoBipMessage `json:"messages"`
}

type infoBipMessage struct {
	From         string               `json:"from"`
	Destinations []infoBipDestination `json:"destinations"`
	Text         string               `json:"text"`
}

type infoBipDestination struct {
	To string `json:"to"`
}

type infoBipResponseBody struct {
	Status string `json:"status"`
}

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

	apiKey := strings.TrimSpace(r.Header.Get("App"))
	if apiKey == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	dec := json.NewDecoder(r.Body)
	var req infoBipRequestBody
	if err := dec.Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	if err := dec.Decode(&struct{}{}); err != io.EOF {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	if len(req.Messages) != 1 {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	msg := req.Messages[0]
	if strings.TrimSpace(msg.From) == "" || strings.TrimSpace(msg.Text) == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	if len(msg.Destinations) != 1 {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	destination := strings.TrimSpace(msg.Destinations[0].To)
	if destination == "" || !isNumericDestination(destination) {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	if strings.Contains(msg.Text, providerFailureToken) {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(infoBipResponseBody{Status: "OK"}); err != nil {
		log.Printf("encode response: %v", err)
	}
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
