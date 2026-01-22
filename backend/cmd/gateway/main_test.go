package main

import (
	"context"
	"encoding/json"
	"gateway"
	"gateway/adapter"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

const defaultProviderTimeout = 30 * time.Second
const defaultProviderConnectTimeout = minProviderConnectTimeout

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

func TestSMSSendInvalidJSON(t *testing.T) {
	gw, err := gateway.New(gateway.Config{
		ProviderCall: func(ctx context.Context, req gateway.SMSRequest) (gateway.ProviderResult, error) {
			return gateway.ProviderResult{Status: "accepted"}, nil
		},
		ProviderTimeout: defaultProviderTimeout,
	})
	if err != nil {
		t.Fatalf("new gateway: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/sms/send", strings.NewReader("{"))
	rec := httptest.NewRecorder()

	handleSMSSend(gw, nil).ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", rec.Code)
	}

	var resp gateway.SMSResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Status != "rejected" {
		t.Fatalf("expected rejected status, got %q", resp.Status)
	}
	if resp.Reason != "invalid_request" {
		t.Fatalf("expected invalid_request, got %q", resp.Reason)
	}
}

func TestSMSSendTrailingJSON(t *testing.T) {
	gw, err := gateway.New(gateway.Config{
		ProviderCall: func(ctx context.Context, req gateway.SMSRequest) (gateway.ProviderResult, error) {
			return gateway.ProviderResult{Status: "accepted"}, nil
		},
		ProviderTimeout: defaultProviderTimeout,
	})
	if err != nil {
		t.Fatalf("new gateway: %v", err)
	}

	body := `{"referenceId":"ref-1","to":"15551234567","message":"hello"} trailing`
	req := httptest.NewRequest(http.MethodPost, "/sms/send", strings.NewReader(body))
	rec := httptest.NewRecorder()

	handleSMSSend(gw, nil).ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", rec.Code)
	}

	var resp gateway.SMSResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Reason != "invalid_request" {
		t.Fatalf("expected invalid_request, got %q", resp.Reason)
	}
}

func TestSMSSendMissingReferenceID(t *testing.T) {
	gw, err := gateway.New(gateway.Config{
		ProviderCall: func(ctx context.Context, req gateway.SMSRequest) (gateway.ProviderResult, error) {
			return gateway.ProviderResult{Status: "accepted"}, nil
		},
		ProviderTimeout: defaultProviderTimeout,
	})
	if err != nil {
		t.Fatalf("new gateway: %v", err)
	}

	body := `{"to":"15551234567","message":"hello"}`
	req := httptest.NewRequest(http.MethodPost, "/sms/send", strings.NewReader(body))
	rec := httptest.NewRecorder()

	handleSMSSend(gw, nil).ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", rec.Code)
	}

	var resp gateway.SMSResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Reason != "invalid_request" {
		t.Fatalf("expected invalid_request, got %q", resp.Reason)
	}
}

func TestSMSSendValidRequestAccepted(t *testing.T) {
	gw, err := gateway.New(gateway.Config{
		ProviderCall: func(ctx context.Context, req gateway.SMSRequest) (gateway.ProviderResult, error) {
			return gateway.ProviderResult{Status: "accepted"}, nil
		},
		ProviderTimeout: defaultProviderTimeout,
	})
	if err != nil {
		t.Fatalf("new gateway: %v", err)
	}

	body := `{"referenceId":"ref-1","to":"15551234567","message":"hello"}`
	req := httptest.NewRequest(http.MethodPost, "/sms/send", strings.NewReader(body))
	rec := httptest.NewRecorder()

	handleSMSSend(gw, nil).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	var resp gateway.SMSResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.ReferenceID != "ref-1" {
		t.Fatalf("expected referenceId ref-1, got %q", resp.ReferenceID)
	}
	if resp.Status != "accepted" {
		t.Fatalf("expected accepted status, got %q", resp.Status)
	}
	if resp.GatewayMessageID == "" {
		t.Fatalf("expected non-empty gatewayMessageId")
	}
}

func TestNewMuxRoutesSMSSend(t *testing.T) {
	gw, err := gateway.New(gateway.Config{
		ProviderCall: func(ctx context.Context, req gateway.SMSRequest) (gateway.ProviderResult, error) {
			return gateway.ProviderResult{Status: "accepted"}, nil
		},
		ProviderTimeout: defaultProviderTimeout,
	})
	if err != nil {
		t.Fatalf("new gateway: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/sms/send", strings.NewReader(""))
	rec := httptest.NewRecorder()

	newMux(gw, nil).ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", rec.Code)
	}

	var resp gateway.SMSResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Reason != "invalid_request" {
		t.Fatalf("expected invalid_request, got %q", resp.Reason)
	}
}

func TestWriteSMSResponse(t *testing.T) {
	rec := httptest.NewRecorder()

	writeSMSResponse(rec, http.StatusBadRequest, gateway.SMSResponse{
		ReferenceID: "ref-1",
		Status:      "rejected",
		Reason:      "invalid_request",
	})

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", rec.Code)
	}
	if got := rec.Header().Get("Content-Type"); got != "application/json" {
		t.Fatalf("expected content-type application/json, got %q", got)
	}

	var resp gateway.SMSResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.ReferenceID != "ref-1" {
		t.Fatalf("expected referenceId ref-1, got %q", resp.ReferenceID)
	}
	if resp.Reason != "invalid_request" {
		t.Fatalf("expected invalid_request, got %q", resp.Reason)
	}
}

func TestSMSSendProviderInvalidMessage(t *testing.T) {
	var got providerRequest
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/sms/send" {
			t.Errorf("expected path /sms/send, got %q", r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Errorf("decode provider request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(providerResponse{Status: "rejected", Reason: "invalid_message"}); err != nil {
			t.Errorf("encode provider response: %v", err)
		}
	}))
	defer provider.Close()

	gw, err := gateway.New(gateway.Config{
		ProviderCall:    adapter.DefaultProviderCall(provider.URL+"/sms/send", defaultProviderConnectTimeout),
		ProviderTimeout: defaultProviderTimeout,
	})
	if err != nil {
		t.Fatalf("new gateway: %v", err)
	}

	body := `{"referenceId":"ref-9","to":"15551234567","message":"hello"}`
	req := httptest.NewRequest(http.MethodPost, "/sms/send", strings.NewReader(body))
	rec := httptest.NewRecorder()

	handleSMSSend(gw, nil).ServeHTTP(rec, req)

	if got.ReferenceID != "ref-9" {
		t.Fatalf("expected provider referenceId ref-9, got %q", got.ReferenceID)
	}
	if got.To != "15551234567" {
		t.Fatalf("expected provider to 15551234567, got %q", got.To)
	}
	if got.Message != "hello" {
		t.Fatalf("expected provider message hello, got %q", got.Message)
	}

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", rec.Code)
	}

	var resp gateway.SMSResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Reason != "invalid_message" {
		t.Fatalf("expected invalid_message, got %q", resp.Reason)
	}
}

func TestSMSSendProviderFailureStatus(t *testing.T) {
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer provider.Close()

	gw, err := gateway.New(gateway.Config{
		ProviderCall:    adapter.DefaultProviderCall(provider.URL+"/sms/send", defaultProviderConnectTimeout),
		ProviderTimeout: defaultProviderTimeout,
	})
	if err != nil {
		t.Fatalf("new gateway: %v", err)
	}

	body := `{"referenceId":"ref-10","to":"15551234567","message":"hello"}`
	req := httptest.NewRequest(http.MethodPost, "/sms/send", strings.NewReader(body))
	rec := httptest.NewRecorder()

	handleSMSSend(gw, nil).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	var resp gateway.SMSResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Reason != "provider_failure" {
		t.Fatalf("expected provider_failure, got %q", resp.Reason)
	}
}
