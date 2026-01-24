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

const fcmProviderConnectTimeout = 2 * time.Second

func TestPushFCMProviderRequestMapping(t *testing.T) {
	var gotMethod string
	var gotContentType string
	var gotAuth string
	var gotBody fcmRequestBody
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotContentType = r.Header.Get("Content-Type")
		gotAuth = r.Header.Get("Authorization")

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

	providerCall := PushFCMProviderCall(provider.URL+"/push/send", "token-1", fcmProviderConnectTimeout)
	resp, err := providerCall(context.Background(), gateway.PushRequest{
		ReferenceID: "ref-1",
		Token:       "device-token",
		Title:       "hello",
		Body:        "world",
		Data: map[string]string{
			"k1": "v1",
		},
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
	if gotAuth != "Bearer token-1" {
		t.Fatalf("expected Authorization Bearer token-1, got %q", gotAuth)
	}
	if gotBody.Message.Token != "device-token" {
		t.Fatalf("expected token device-token, got %q", gotBody.Message.Token)
	}
	if gotBody.Message.Notification == nil {
		t.Fatal("expected notification payload")
	}
	if gotBody.Message.Notification.Title != "hello" {
		t.Fatalf("expected title hello, got %q", gotBody.Message.Notification.Title)
	}
	if gotBody.Message.Notification.Body != "world" {
		t.Fatalf("expected body world, got %q", gotBody.Message.Notification.Body)
	}
	if gotBody.Message.Data["k1"] != "v1" {
		t.Fatalf("expected data k1=v1, got %q", gotBody.Message.Data["k1"])
	}
}

func TestPushFCMProviderNon2xx(t *testing.T) {
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer provider.Close()

	providerCall := PushFCMProviderCall(provider.URL, "token-2", fcmProviderConnectTimeout)
	_, err := providerCall(context.Background(), gateway.PushRequest{
		ReferenceID: "ref-2",
		Token:       "device-token",
		Body:        "hello",
	})
	if err == nil {
		t.Fatal("expected error for non-2xx response")
	}
}

func TestPushFCMProviderMissingURL(t *testing.T) {
	providerCall := PushFCMProviderCall("", "token", fcmProviderConnectTimeout)
	if providerCall != nil {
		t.Fatal("expected nil ProviderCall for empty URL")
	}
}
