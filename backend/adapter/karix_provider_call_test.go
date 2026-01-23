package adapter

import (
	"context"
	"gateway"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

const karixProviderConnectTimeout = 2 * time.Second

func TestSmsKarixProviderRequestMapping(t *testing.T) {
	var gotMethod string
	var gotPath string
	var gotQuery url.Values
	var gotRawQuery string
	var gotBody []byte
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotQuery = r.URL.Query()
		gotRawQuery = r.URL.RawQuery

		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read body: %v", err)
			return
		}
		gotBody = body

		w.WriteHeader(http.StatusNoContent)
	}))
	defer provider.Close()

	providerCall := SmsKarixProviderCall(provider.URL+"/sms/send", "key-1", "v1", "sender-1", karixProviderConnectTimeout)
	resp, err := providerCall(context.Background(), gateway.SMSRequest{
		ReferenceID: "ref-1",
		To:          "15551234567",
		Message:     "hello world",
	})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if resp.Status != "accepted" {
		t.Fatalf("expected accepted status, got %q", resp.Status)
	}

	if gotMethod != http.MethodGet {
		t.Fatalf("expected method GET, got %q", gotMethod)
	}
	if gotPath != "/sms/send" {
		t.Fatalf("expected path /sms/send, got %q", gotPath)
	}
	if len(gotQuery) != 6 {
		t.Fatalf("expected 6 query params, got %d", len(gotQuery))
	}
	if gotQuery.Get("ver") != "v1" {
		t.Fatalf("expected ver v1, got %q", gotQuery.Get("ver"))
	}
	if gotQuery.Get("key") != "key-1" {
		t.Fatalf("expected key key-1, got %q", gotQuery.Get("key"))
	}
	if gotQuery.Get("encrpt") != "0" {
		t.Fatalf("expected encrpt 0, got %q", gotQuery.Get("encrpt"))
	}
	if gotQuery.Get("dest") != "15551234567" {
		t.Fatalf("expected dest 15551234567, got %q", gotQuery.Get("dest"))
	}
	if gotQuery.Get("send") != "sender-1" {
		t.Fatalf("expected send sender-1, got %q", gotQuery.Get("send"))
	}
	if gotQuery.Get("text") != "hello world" {
		t.Fatalf("expected text hello world, got %q", gotQuery.Get("text"))
	}
	if strings.Contains(gotRawQuery, " ") {
		t.Fatalf("expected encoded query string, got %q", gotRawQuery)
	}
	if len(gotBody) != 0 {
		t.Fatalf("expected empty request body, got %q", string(gotBody))
	}
}

func TestSmsKarixProviderRequestMappingWithExistingQuery(t *testing.T) {
	var gotPath string
	var gotQuery url.Values
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotQuery = r.URL.Query()
		w.WriteHeader(http.StatusNoContent)
	}))
	defer provider.Close()

	providerCall := SmsKarixProviderCall(provider.URL+"/sms/send?existing=1", "key-2", "v2", "sender-2", karixProviderConnectTimeout)
	resp, err := providerCall(context.Background(), gateway.SMSRequest{
		ReferenceID: "ref-2",
		To:          "15551234567",
		Message:     "hello",
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
	if gotQuery.Get("existing") != "1" {
		t.Fatalf("expected existing query param, got %q", gotQuery.Get("existing"))
	}
	if gotQuery.Get("ver") != "v2" {
		t.Fatalf("expected ver v2, got %q", gotQuery.Get("ver"))
	}
	if gotQuery.Get("key") != "key-2" {
		t.Fatalf("expected key key-2, got %q", gotQuery.Get("key"))
	}
	if gotQuery.Get("encrpt") != "0" {
		t.Fatalf("expected encrpt 0, got %q", gotQuery.Get("encrpt"))
	}
	if gotQuery.Get("dest") != "15551234567" {
		t.Fatalf("expected dest 15551234567, got %q", gotQuery.Get("dest"))
	}
	if gotQuery.Get("send") != "sender-2" {
		t.Fatalf("expected send sender-2, got %q", gotQuery.Get("send"))
	}
	if gotQuery.Get("text") != "hello" {
		t.Fatalf("expected text hello, got %q", gotQuery.Get("text"))
	}
	if len(gotQuery) != 7 {
		t.Fatalf("expected 7 query params, got %d", len(gotQuery))
	}
}

func TestSmsKarixProviderNon2xx(t *testing.T) {
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer provider.Close()

	providerCall := SmsKarixProviderCall(provider.URL, "key-3", "v3", "sender-3", karixProviderConnectTimeout)
	_, err := providerCall(context.Background(), gateway.SMSRequest{
		ReferenceID: "ref-3",
		To:          "15551234567",
		Message:     "hello",
	})
	if err == nil {
		t.Fatal("expected error for non-2xx response")
	}
}

func TestSmsKarixProviderMissingURL(t *testing.T) {
	providerCall := SmsKarixProviderCall("", "key", "v1", "sender", karixProviderConnectTimeout)
	if providerCall != nil {
		t.Fatal("expected nil ProviderCall for empty URL")
	}
}
