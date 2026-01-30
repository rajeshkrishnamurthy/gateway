package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"gateway/submission"
	"gateway/submissionmanager"
)

func TestExecutorParsesOutcomeOn2xx(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/sms/send" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"rejected","reason":"invalid_request"}`))
	}))
	defer server.Close()

	exec := newGatewayExecutor(server.Client())
	outcome, err := exec(context.Background(), submissionmanager.AttemptInput{
		GatewayType: submission.GatewaySMS,
		GatewayURL:  server.URL,
		Payload:     []byte(`{"referenceId":"ref-1"}`),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if outcome.Status != "rejected" || outcome.Reason != "invalid_request" {
		t.Fatalf("unexpected outcome: %+v", outcome)
	}
}

func TestExecutorTreatsNon2xxAsError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("error"))
	}))
	defer server.Close()

	exec := newGatewayExecutor(server.Client())
	_, err := exec(context.Background(), submissionmanager.AttemptInput{
		GatewayType: submission.GatewaySMS,
		GatewayURL:  server.URL,
		Payload:     []byte(`{"referenceId":"ref-1"}`),
	})
	if err == nil {
		t.Fatalf("expected error")
	}
	if !strings.Contains(err.Error(), "status 500") {
		t.Fatalf("expected status in error, got %v", err)
	}
}

func TestExecutorRejectsMalformedJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("not-json"))
	}))
	defer server.Close()

	exec := newGatewayExecutor(server.Client())
	_, err := exec(context.Background(), submissionmanager.AttemptInput{
		GatewayType: submission.GatewaySMS,
		GatewayURL:  server.URL,
		Payload:     []byte(`{"referenceId":"ref-1"}`),
	})
	if err == nil {
		t.Fatalf("expected error")
	}
	if !strings.Contains(err.Error(), "decode gateway response") {
		t.Fatalf("expected decode error, got %v", err)
	}
}
