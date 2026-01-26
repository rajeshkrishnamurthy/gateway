package main

import (
	"encoding/json"
	"flag"
	"io"
	"log"
	"math/rand"
	"net/http"
	"strings"
	"time"
)

const (
	maxBodyBytes          = 16 << 10
	providerFailureToken  = "FAIL"
	modelProviderID       = "abc123"
	modelProviderEndpoint = "/sms/send"
	minProcessingDelay    = 50 * time.Millisecond
	maxProcessingDelay    = 2 * time.Second
)

var addr = flag.String("addr", ":9091", "HTTP listen address")

type modelProviderRequestBody struct {
	Destination string `json:"destination"`
	Text        string `json:"text"`
}

type modelProviderResponseBody struct {
	Status     string `json:"status"`
	ProviderID string `json:"provider_id"`
}

type modelProviderErrorBody struct {
	Error string `json:"error"`
}

func main() {
	flag.Parse()
	rand.Seed(time.Now().UnixNano())

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", handleHealthz)
	mux.HandleFunc("/readyz", handleReadyz)
	mux.HandleFunc(modelProviderEndpoint, handleSend)

	log.Printf("listening on %s", *addr)
	log.Fatal(http.ListenAndServe(*addr, mux))
}

func handleHealthz(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

func handleReadyz(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

func handleSend(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	referenceID := r.Header.Get("X-Request-Id")
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)

	dec := json.NewDecoder(r.Body)
	var req modelProviderRequestBody
	if err := dec.Decode(&req); err != nil {
		writeError(w, referenceID, "INVALID_MESSAGE")
		return
	}
	if err := dec.Decode(&struct{}{}); err != io.EOF {
		writeError(w, referenceID, "INVALID_MESSAGE")
		return
	}

	delay := minProcessingDelay
	if maxProcessingDelay > minProcessingDelay {
		delay += time.Duration(rand.Int63n(int64(maxProcessingDelay - minProcessingDelay)))
	}
	// Simulate provider latency for manual end-to-end testing. TODO: remove this delay.
	time.Sleep(delay)

	if strings.TrimSpace(req.Destination) == "" || !isNumericDestination(req.Destination) {
		writeError(w, referenceID, "INVALID_RECIPIENT")
		return
	}
	if strings.TrimSpace(req.Text) == "" {
		writeError(w, referenceID, "INVALID_MESSAGE")
		return
	}
	if strings.Contains(req.Text, providerFailureToken) {
		log.Printf("provider decision referenceId=%q status=provider_failure", referenceID)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	log.Printf("provider decision referenceId=%q status=accepted providerId=%q", referenceID, modelProviderID)
	writeSuccess(w)
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

func writeError(w http.ResponseWriter, referenceID, code string) {
	log.Printf("provider decision referenceId=%q status=rejected error=%q", referenceID, code)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusBadRequest)
	if err := json.NewEncoder(w).Encode(modelProviderErrorBody{Error: code}); err != nil {
		log.Printf("encode response: %v", err)
	}
}

func writeSuccess(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(modelProviderResponseBody{
		Status:     "OK",
		ProviderID: modelProviderID,
	}); err != nil {
		log.Printf("encode response: %v", err)
	}
}
