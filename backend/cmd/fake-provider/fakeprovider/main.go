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
	maxBodyBytes  = 16 << 10
	maxMessageLen = 20
	failSuffix    = "FAIL"
)

var addr = flag.String("addr", ":9090", "HTTP listen address")

type providerRequest struct {
	ReferenceID string `json:"referenceId"`
	To          string `json:"to"`
	Message     string `json:"message"`
	TenantID    string `json:"tenantId,omitempty"`
}

type providerResponse struct {
	Status string `json:"status"`
	Reason string `json:"reason,omitempty"`
}

func main() {
	flag.Parse()

	mux := http.NewServeMux()
	mux.HandleFunc("/sms/send", handleSend)

	log.Printf("listening on %s", *addr)
	log.Fatal(http.ListenAndServe(*addr, mux))
}

func handleSend(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)

	dec := json.NewDecoder(r.Body)
	var req providerRequest
	if err := dec.Decode(&req); err != nil {
		writeProviderResponse(w, http.StatusBadRequest, providerResponse{
			Status: "rejected",
			Reason: "provider_failure",
		})
		return
	}
	if err := dec.Decode(&struct{}{}); err != io.EOF {
		writeProviderResponse(w, http.StatusBadRequest, providerResponse{
			Status: "rejected",
			Reason: "provider_failure",
		})
		return
	}

	if strings.HasSuffix(req.ReferenceID, failSuffix) {
		writeProviderResponse(w, http.StatusInternalServerError, providerResponse{
			Status: "rejected",
			Reason: "provider_failure",
		})
		return
	}
	if !isNumericDestination(req.To) {
		writeProviderResponse(w, http.StatusOK, providerResponse{
			Status: "rejected",
			Reason: "invalid_recipient",
		})
		return
	}
	if strings.TrimSpace(req.Message) == "" {
		writeProviderResponse(w, http.StatusOK, providerResponse{
			Status: "rejected",
			Reason: "invalid_message",
		})
		return
	}
	if len(req.Message) > maxMessageLen {
		writeProviderResponse(w, http.StatusOK, providerResponse{
			Status: "rejected",
			Reason: "invalid_message",
		})
		return
	}

	writeProviderResponse(w, http.StatusOK, providerResponse{
		Status: "accepted",
	})
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

func writeProviderResponse(w http.ResponseWriter, status int, resp providerResponse) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		log.Printf("encode response: %v", err)
	}
}
