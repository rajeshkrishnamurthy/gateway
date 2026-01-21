package adapter

import (
	"context"
	"encoding/json"
	"gateway"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

const defaultProviderConnectTimeout = 2 * time.Second

func TestModelProviderRequestMapping(t *testing.T) {
	var gotPath string
	var gotContentType string
	var gotRequestID string
	var gotBody map[string]string
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotContentType = r.Header.Get("Content-Type")
		gotRequestID = r.Header.Get("X-Request-Id")

		dec := json.NewDecoder(r.Body)
		if err := dec.Decode(&gotBody); err != nil {
			t.Errorf("decode body: %v", err)
			return
		}
		if err := dec.Decode(&struct{}{}); err != io.EOF {
			t.Errorf("expected EOF after body, got %v", err)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]string{"status": "OK", "provider_id": "abc123"}); err != nil {
			t.Errorf("encode response: %v", err)
			return
		}
	}))
	defer provider.Close()

	providerCall := ModelProviderCall(provider.URL+"/sms/send", defaultProviderConnectTimeout)
	resp, err := providerCall(context.Background(), gateway.SMSRequest{
		ReferenceID: "ref-1",
		To:          "15551234567",
		Message:     "hello",
		TenantID:    "tenant-x",
	})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if resp.Status != "accepted" {
		t.Fatalf("expected accepted status, got %q", resp.Status)
	}

	if gotPath != "/sms/send" {
		t.Fatalf("expected path /sms/send, got %q", gotPath)
	}
	if gotContentType != "application/json" {
		t.Fatalf("expected content-type application/json, got %q", gotContentType)
	}
	if gotRequestID != "ref-1" {
		t.Fatalf("expected X-Request-Id ref-1, got %q", gotRequestID)
	}
	if gotBody["destination"] != "15551234567" {
		t.Fatalf("expected destination 15551234567, got %q", gotBody["destination"])
	}
	if gotBody["text"] != "hello" {
		t.Fatalf("expected text hello, got %q", gotBody["text"])
	}
	if _, ok := gotBody["tenantId"]; ok {
		t.Fatalf("unexpected tenantId in payload")
	}
	if _, ok := gotBody["referenceId"]; ok {
		t.Fatalf("unexpected referenceId in payload")
	}
	if len(gotBody) != 2 {
		t.Fatalf("expected 2 fields in payload, got %d", len(gotBody))
	}
}

func TestModelProviderInvalidRecipient(t *testing.T) {
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		if err := json.NewEncoder(w).Encode(map[string]string{"error": "INVALID_RECIPIENT"}); err != nil {
			t.Errorf("encode response: %v", err)
			return
		}
	}))
	defer provider.Close()

	providerCall := ModelProviderCall(provider.URL, defaultProviderConnectTimeout)
	resp, err := providerCall(context.Background(), gateway.SMSRequest{
		ReferenceID: "ref-2",
		To:          "abc",
		Message:     "hello",
	})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if resp.Status != "rejected" {
		t.Fatalf("expected rejected status, got %q", resp.Status)
	}
	if resp.Reason != "invalid_recipient" {
		t.Fatalf("expected invalid_recipient, got %q", resp.Reason)
	}
}

func TestModelProviderInvalidMessage(t *testing.T) {
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		if err := json.NewEncoder(w).Encode(map[string]string{"error": "INVALID_MESSAGE"}); err != nil {
			t.Errorf("encode response: %v", err)
			return
		}
	}))
	defer provider.Close()

	providerCall := ModelProviderCall(provider.URL, defaultProviderConnectTimeout)
	resp, err := providerCall(context.Background(), gateway.SMSRequest{
		ReferenceID: "ref-3",
		To:          "15551234567",
		Message:     strings.Repeat("x", 21),
	})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if resp.Status != "rejected" {
		t.Fatalf("expected rejected status, got %q", resp.Status)
	}
	if resp.Reason != "invalid_message" {
		t.Fatalf("expected invalid_message, got %q", resp.Reason)
	}
}

func TestModelProviderUnknownError(t *testing.T) {
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		if err := json.NewEncoder(w).Encode(map[string]string{"error": "OTHER"}); err != nil {
			t.Errorf("encode response: %v", err)
			return
		}
	}))
	defer provider.Close()

	providerCall := ModelProviderCall(provider.URL, defaultProviderConnectTimeout)
	_, err := providerCall(context.Background(), gateway.SMSRequest{
		ReferenceID: "ref-4",
		To:          "15551234567",
		Message:     "hello",
	})
	if err == nil {
		t.Fatal("expected error for unknown provider error code")
	}
}

func TestModelProviderMalformedAcceptedResponse(t *testing.T) {
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status":"OK"}`))
	}))
	defer provider.Close()

	providerCall := ModelProviderCall(provider.URL, defaultProviderConnectTimeout)
	_, err := providerCall(context.Background(), gateway.SMSRequest{
		ReferenceID: "ref-5",
		To:          "15551234567",
		Message:     "hello",
	})
	if err == nil {
		t.Fatal("expected error for missing provider_id")
	}
}

func TestModelProviderFailureStatus(t *testing.T) {
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer provider.Close()

	providerCall := ModelProviderCall(provider.URL, defaultProviderConnectTimeout)
	_, err := providerCall(context.Background(), gateway.SMSRequest{
		ReferenceID: "ref-6",
		To:          "15551234567",
		Message:     "hello",
	})
	if err == nil {
		t.Fatal("expected error for provider failure status")
	}
}
