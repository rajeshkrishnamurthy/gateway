package main

import (
	"encoding/json"
	"flag"
	"io"
	"log"
	"net/http"
	"sync"
	"time"
)

var addr = flag.String("addr", ":9999", "HTTP listen address")

type webhookEvent struct {
	EventID   string       `json:"eventId"`
	EventType string       `json:"eventType"`
	Intent    webhookIntent `json:"intent"`
}

type webhookIntent struct {
	IntentID        string `json:"intentId"`
	SubmissionTarget string `json:"submissionTarget"`
	Status          string `json:"status"`
	RejectedReason  string `json:"rejectedReason,omitempty"`
	ExhaustedReason string `json:"exhaustedReason,omitempty"`
}

type lastWebhook struct {
	ReceivedAt time.Time    `json:"receivedAt"`
	Event      webhookEvent `json:"event"`
}

type webhookStore struct {
	mu   sync.Mutex
	last lastWebhook
	ok   bool
}

func main() {
	flag.Parse()

	store := &webhookStore{}
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", handleHealthz)
	mux.HandleFunc("/readyz", handleReadyz)
	mux.HandleFunc("/webhook", store.handleWebhook)
	mux.HandleFunc("/last", store.handleLast)

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

func (s *webhookStore) handleWebhook(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	body, _ := io.ReadAll(r.Body)
	var event webhookEvent
	if err := json.Unmarshal(body, &event); err != nil {
		log.Printf("webhook decode failed: %v", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	s.mu.Lock()
	s.last = lastWebhook{ReceivedAt: time.Now().UTC(), Event: event}
	s.ok = true
	s.mu.Unlock()
	log.Printf("webhook received eventType=%q intentId=%q status=%q", event.EventType, event.Intent.IntentID, event.Intent.Status)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

func (s *webhookStore) handleLast(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	s.mu.Lock()
	last := s.last
	ok := s.ok
	s.mu.Unlock()
	if !ok {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(last); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
}
