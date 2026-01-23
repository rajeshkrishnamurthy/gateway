package adapter

import (
	"context"
	"gateway"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"
)

const sms24x7ProviderConnectTimeout = 2 * time.Second

func TestSms24X7ProviderRequestMapping(t *testing.T) {
	var gotMethod string
	var gotPath string
	var gotContentType string
	var gotRequestID string
	var gotQuery url.Values
	var gotBody []byte
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotContentType = r.Header.Get("Content-Type")
		gotRequestID = r.Header.Get("X-Request-Id")
		gotQuery = r.URL.Query()

		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read body: %v", err)
			return
		}
		gotBody = body

		w.WriteHeader(http.StatusNoContent)
	}))
	defer provider.Close()

	providerCall := Sms24X7ProviderCall(provider.URL+"/sms/send", "key-1", "svc-1", "sender-1", sms24x7ProviderConnectTimeout)
	resp, err := providerCall(context.Background(), gateway.SMSRequest{
		ReferenceID: "ref-1",
		To:          "1555+123 456",
		Message:     "hello+world test",
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
	if gotPath != "/sms/send" {
		t.Fatalf("expected path /sms/send, got %q", gotPath)
	}
	if gotContentType != "" {
		t.Fatalf("expected empty content-type, got %q", gotContentType)
	}
	if gotRequestID != "" {
		t.Fatalf("expected empty X-Request-Id, got %q", gotRequestID)
	}
	if len(gotQuery) != 5 {
		t.Fatalf("expected 5 query params, got %d", len(gotQuery))
	}
	if gotQuery.Get("ApiKey") != "key-1" {
		t.Fatalf("expected ApiKey key-1, got %q", gotQuery.Get("ApiKey"))
	}
	if gotQuery.Get("ServiceName") != "svc-1" {
		t.Fatalf("expected ServiceName svc-1, got %q", gotQuery.Get("ServiceName"))
	}
	if gotQuery.Get("SenderId") != "sender-1" {
		t.Fatalf("expected SenderId sender-1, got %q", gotQuery.Get("SenderId"))
	}
	if gotQuery.Get("MobileNo") != "1555+123 456" {
		t.Fatalf("expected MobileNo 1555+123 456, got %q", gotQuery.Get("MobileNo"))
	}
	if gotQuery.Get("Message") != "hello+world test" {
		t.Fatalf("expected Message hello+world test, got %q", gotQuery.Get("Message"))
	}
	if len(gotBody) != 0 {
		t.Fatalf("expected empty request body, got %q", string(gotBody))
	}
}

func TestSms24X7ProviderNon2xx(t *testing.T) {
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer provider.Close()

	providerCall := Sms24X7ProviderCall(provider.URL, "key-2", "svc-2", "sender-2", sms24x7ProviderConnectTimeout)
	_, err := providerCall(context.Background(), gateway.SMSRequest{
		ReferenceID: "ref-2",
		To:          "15551234567",
		Message:     "hello",
	})
	if err == nil {
		t.Fatal("expected error for non-2xx response")
	}
}

func TestSms24X7ProviderMissingURL(t *testing.T) {
	providerCall := Sms24X7ProviderCall("", "key", "svc", "sender", sms24x7ProviderConnectTimeout)
	if providerCall != nil {
		t.Fatal("expected nil ProviderCall for empty URL")
	}
}

func TestSms24X7ProviderRequestMappingWithExistingQuery(t *testing.T) {
	var gotPath string
	var gotQuery url.Values
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotQuery = r.URL.Query()
		w.WriteHeader(http.StatusNoContent)
	}))
	defer provider.Close()

	providerCall := Sms24X7ProviderCall(provider.URL+"/sms/send?existing=1", "key-3", "svc-3", "sender-3", sms24x7ProviderConnectTimeout)
	resp, err := providerCall(context.Background(), gateway.SMSRequest{
		ReferenceID: "ref-3",
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
	if gotQuery.Get("ApiKey") != "key-3" {
		t.Fatalf("expected ApiKey key-3, got %q", gotQuery.Get("ApiKey"))
	}
	if gotQuery.Get("ServiceName") != "svc-3" {
		t.Fatalf("expected ServiceName svc-3, got %q", gotQuery.Get("ServiceName"))
	}
	if gotQuery.Get("SenderId") != "sender-3" {
		t.Fatalf("expected SenderId sender-3, got %q", gotQuery.Get("SenderId"))
	}
	if gotQuery.Get("MobileNo") != "15551234567" {
		t.Fatalf("expected MobileNo 15551234567, got %q", gotQuery.Get("MobileNo"))
	}
	if gotQuery.Get("Message") != "hello" {
		t.Fatalf("expected Message hello, got %q", gotQuery.Get("Message"))
	}
	if len(gotQuery) != 6 {
		t.Fatalf("expected 6 query params, got %d", len(gotQuery))
	}
}
