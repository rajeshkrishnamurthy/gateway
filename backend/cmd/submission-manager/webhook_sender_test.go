package main

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"gateway/submissionmanager"
)

func TestWebhookSenderSignsPayload(t *testing.T) {
	t.Setenv("WEBHOOK_SECRET", "secret-value")
	t.Setenv("WEBHOOK_AUTH", "Bearer abc")

	var gotSignature string
	var gotAuth string
	var gotContentType string
	var gotBody []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotSignature = r.Header.Get("X-Setu-Signature")
		gotAuth = r.Header.Get("Authorization")
		gotContentType = r.Header.Get("Content-Type")
		body, _ := io.ReadAll(r.Body)
		gotBody = body
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	sender := newWebhookSender(server.Client())
	delivery := submissionmanager.WebhookDelivery{
		URL:       server.URL,
		SecretEnv: "WEBHOOK_SECRET",
		HeadersEnv: map[string]string{
			"Authorization": "WEBHOOK_AUTH",
		},
		Body: []byte(`{"ok":true}`),
	}
	if err := sender(context.Background(), delivery); err != nil {
		t.Fatalf("send webhook: %v", err)
	}

	mac := hmac.New(sha256.New, []byte("secret-value"))
	_, _ = mac.Write(gotBody)
	wantSignature := hex.EncodeToString(mac.Sum(nil))
	if gotSignature != wantSignature {
		t.Fatalf("expected signature %q, got %q", wantSignature, gotSignature)
	}
	if gotAuth != "Bearer abc" {
		t.Fatalf("expected auth header, got %q", gotAuth)
	}
	if gotContentType != "application/json" {
		t.Fatalf("expected content type application/json, got %q", gotContentType)
	}
}

func TestWebhookSenderMissingEnvFails(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	sender := newWebhookSender(server.Client())
	delivery := submissionmanager.WebhookDelivery{
		URL:       server.URL,
		SecretEnv: "MISSING_SECRET",
		Body:      []byte(`{"ok":true}`),
	}
	if err := sender(context.Background(), delivery); err == nil {
		t.Fatal("expected error")
	}
}
