package adapter

import (
	"context"
	"encoding/json"
	"gateway"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

const infoBipProviderConnectTimeout = 2 * time.Second

func TestSmsInfoBipProviderRequestMapping(t *testing.T) {
	var gotMethod string
	var gotContentType string
	var gotAPIKey string
	var gotRequestID string
	var gotBody infoBipRequestBody
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotContentType = r.Header.Get("Content-Type")
		gotAPIKey = r.Header.Get("App")
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

		w.WriteHeader(http.StatusNoContent)
	}))
	defer provider.Close()

	providerCall := SmsInfoBipProviderCall(provider.URL+"/sms/send", "key-1", "sender-1", infoBipProviderConnectTimeout)
	resp, err := providerCall(context.Background(), gateway.SMSRequest{
		ReferenceID: "ref-1",
		To:          "15551234567",
		Message:     "hello",
	})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if resp.Status != "accepted" {
		t.Fatalf("expected accepted status, got %q", resp.Status)
	}

	if gotMethod != http.MethodPost {
		t.Fatalf("expected method POST, got %q", gotMethod)
	}
	if gotContentType != "application/json" {
		t.Fatalf("expected content-type application/json, got %q", gotContentType)
	}
	if gotAPIKey != "key-1" {
		t.Fatalf("expected App key-1, got %q", gotAPIKey)
	}
	if gotRequestID != "" {
		t.Fatalf("expected empty X-Request-Id, got %q", gotRequestID)
	}
	if len(gotBody.Messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(gotBody.Messages))
	}
	msg := gotBody.Messages[0]
	if msg.From != "sender-1" {
		t.Fatalf("expected from sender-1, got %q", msg.From)
	}
	if msg.Text != "hello" {
		t.Fatalf("expected text hello, got %q", msg.Text)
	}
	if len(msg.Destinations) != 1 {
		t.Fatalf("expected 1 destination, got %d", len(msg.Destinations))
	}
	if msg.Destinations[0].To != "15551234567" {
		t.Fatalf("expected to 15551234567, got %q", msg.Destinations[0].To)
	}
}

func TestSmsInfoBipProviderNon2xx(t *testing.T) {
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer provider.Close()

	providerCall := SmsInfoBipProviderCall(provider.URL, "key-2", "sender-2", infoBipProviderConnectTimeout)
	_, err := providerCall(context.Background(), gateway.SMSRequest{
		ReferenceID: "ref-2",
		To:          "15551234567",
		Message:     "hello",
	})
	if err == nil {
		t.Fatal("expected error for non-2xx response")
	}
}

func TestSmsInfoBipProviderMissingURL(t *testing.T) {
	providerCall := SmsInfoBipProviderCall("", "key", "sender", infoBipProviderConnectTimeout)
	if providerCall != nil {
		t.Fatal("expected nil ProviderCall for empty URL")
	}
}
