package gateway

import (
	"context"
	"errors"
	"testing"
)

func TestNewMissingProvider(t *testing.T) {
	_, err := New(Config{})
	if err == nil {
		t.Fatal("expected error for missing provider")
	}
}

func TestNewUnknownProvider(t *testing.T) {
	_, err := New(Config{Provider: "unknown"})
	if err == nil {
		t.Fatal("expected error for unknown provider")
	}
}

func TestSendSMSMissingReferenceID(t *testing.T) {
	gw, err := New(Config{Provider: "24x7"})
	if err != nil {
		t.Fatalf("new gateway: %v", err)
	}

	resp, err := gw.SendSMS(context.Background(), SMSRequest{
		To:      "15551234567",
		Message: "hello",
	})
	if !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("expected ErrInvalidRequest, got %v", err)
	}
	if resp.Status != "rejected" {
		t.Fatalf("expected rejected status, got %q", resp.Status)
	}
	if resp.Reason != "missing_reference_id" {
		t.Fatalf("expected missing_reference_id, got %q", resp.Reason)
	}
}

func TestSendSMSMissingTo(t *testing.T) {
	gw, err := New(Config{Provider: "24x7"})
	if err != nil {
		t.Fatalf("new gateway: %v", err)
	}

	resp, err := gw.SendSMS(context.Background(), SMSRequest{
		ReferenceID: "ref-1",
		Message:     "hello",
	})
	if !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("expected ErrInvalidRequest, got %v", err)
	}
	if resp.ReferenceID != "ref-1" {
		t.Fatalf("expected referenceId ref-1, got %q", resp.ReferenceID)
	}
	if resp.Reason != "missing_to" {
		t.Fatalf("expected missing_to, got %q", resp.Reason)
	}
}

func TestSendSMSMissingMessage(t *testing.T) {
	gw, err := New(Config{Provider: "24x7"})
	if err != nil {
		t.Fatalf("new gateway: %v", err)
	}

	resp, err := gw.SendSMS(context.Background(), SMSRequest{
		ReferenceID: "ref-2",
		To:          "15551234567",
	})
	if !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("expected ErrInvalidRequest, got %v", err)
	}
	if resp.ReferenceID != "ref-2" {
		t.Fatalf("expected referenceId ref-2, got %q", resp.ReferenceID)
	}
	if resp.Reason != "missing_message" {
		t.Fatalf("expected missing_message, got %q", resp.Reason)
	}
}

func TestSendSMSValidRequestAccepted(t *testing.T) {
	gw, err := New(Config{Provider: "24X7"})
	if err != nil {
		t.Fatalf("new gateway: %v", err)
	}

	resp, err := gw.SendSMS(context.Background(), SMSRequest{
		ReferenceID: "ref-3",
		To:          "15551234567",
		Message:     "hello",
	})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if resp.ReferenceID != "ref-3" {
		t.Fatalf("expected referenceId ref-3, got %q", resp.ReferenceID)
	}
	if resp.Status != "accepted" {
		t.Fatalf("expected accepted status, got %q", resp.Status)
	}
	if resp.GatewayMessageID == "" {
		t.Fatalf("expected non-empty gatewayMessageId")
	}
	if resp.Reason != "" {
		t.Fatalf("expected empty reason, got %q", resp.Reason)
	}
}

func TestSendSMSContextCanceled(t *testing.T) {
	gw, err := New(Config{Provider: "24x7"})
	if err != nil {
		t.Fatalf("new gateway: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	resp, err := gw.SendSMS(ctx, SMSRequest{
		ReferenceID: "ref-4",
		To:          "15551234567",
		Message:     "hello",
	})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if resp.ReferenceID != "ref-4" {
		t.Fatalf("expected referenceId ref-4, got %q", resp.ReferenceID)
	}
	if resp.Status != "rejected" {
		t.Fatalf("expected rejected status, got %q", resp.Status)
	}
	if resp.Reason != "gateway_unavailable" {
		t.Fatalf("expected gateway_unavailable, got %q", resp.Reason)
	}
}
