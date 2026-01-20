package main

import (
	"encoding/json"
	"gateway"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSMSSendInvalidJSON(t *testing.T) {
	gw, err := gateway.New(gateway.Config{Provider: "24x7"})
	if err != nil {
		t.Fatalf("new gateway: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/sms/send", strings.NewReader("{"))
	rec := httptest.NewRecorder()

	handleSMSSend(gw).ServeHTTP(rec, req)

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
	gw, err := gateway.New(gateway.Config{Provider: "24x7"})
	if err != nil {
		t.Fatalf("new gateway: %v", err)
	}

	body := `{"referenceId":"ref-1","to":"15551234567","message":"hello"} trailing`
	req := httptest.NewRequest(http.MethodPost, "/sms/send", strings.NewReader(body))
	rec := httptest.NewRecorder()

	handleSMSSend(gw).ServeHTTP(rec, req)

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
	gw, err := gateway.New(gateway.Config{Provider: "24x7"})
	if err != nil {
		t.Fatalf("new gateway: %v", err)
	}

	body := `{"to":"15551234567","message":"hello"}`
	req := httptest.NewRequest(http.MethodPost, "/sms/send", strings.NewReader(body))
	rec := httptest.NewRecorder()

	handleSMSSend(gw).ServeHTTP(rec, req)

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
	gw, err := gateway.New(gateway.Config{Provider: "24x7"})
	if err != nil {
		t.Fatalf("new gateway: %v", err)
	}

	body := `{"referenceId":"ref-1","to":"15551234567","message":"hello"}`
	req := httptest.NewRequest(http.MethodPost, "/sms/send", strings.NewReader(body))
	rec := httptest.NewRecorder()

	handleSMSSend(gw).ServeHTTP(rec, req)

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
	gw, err := gateway.New(gateway.Config{Provider: "24x7"})
	if err != nil {
		t.Fatalf("new gateway: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/sms/send", strings.NewReader(""))
	rec := httptest.NewRecorder()

	newMux(gw).ServeHTTP(rec, req)

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
