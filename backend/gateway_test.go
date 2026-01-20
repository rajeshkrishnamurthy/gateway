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
	if resp.Reason != "invalid_request" {
		t.Fatalf("expected invalid_request, got %q", resp.Reason)
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
	if resp.Reason != "invalid_request" {
		t.Fatalf("expected invalid_request, got %q", resp.Reason)
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
	if resp.Reason != "invalid_request" {
		t.Fatalf("expected invalid_request, got %q", resp.Reason)
	}
}

func TestSendSMSInvalidRecipient(t *testing.T) {
	gw, err := New(Config{Provider: "24x7"})
	if err != nil {
		t.Fatalf("new gateway: %v", err)
	}

	resp, err := gw.SendSMS(context.Background(), SMSRequest{
		ReferenceID: "ref-3",
		To:          "abc123",
		Message:     "hello",
	})
	if !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("expected ErrInvalidRequest, got %v", err)
	}
	if resp.ReferenceID != "ref-3" {
		t.Fatalf("expected referenceId ref-3, got %q", resp.ReferenceID)
	}
	if resp.Reason != "invalid_recipient" {
		t.Fatalf("expected invalid_recipient, got %q", resp.Reason)
	}
}

func TestSendSMSInvalidMessage(t *testing.T) {
	gw, err := New(Config{Provider: "24x7"})
	if err != nil {
		t.Fatalf("new gateway: %v", err)
	}

	resp, err := gw.SendSMS(context.Background(), SMSRequest{
		ReferenceID: "ref-4",
		To:          "15551234567",
		Message:     "   ",
	})
	if !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("expected ErrInvalidRequest, got %v", err)
	}
	if resp.ReferenceID != "ref-4" {
		t.Fatalf("expected referenceId ref-4, got %q", resp.ReferenceID)
	}
	if resp.Reason != "invalid_message" {
		t.Fatalf("expected invalid_message, got %q", resp.Reason)
	}
}

func TestSendSMSDuplicateReference(t *testing.T) {
	gw, err := New(Config{Provider: "24x7"})
	if err != nil {
		t.Fatalf("new gateway: %v", err)
	}

	_, err = gw.SendSMS(context.Background(), SMSRequest{
		ReferenceID: "ref-5",
		To:          "15551234567",
		Message:     "hello",
	})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}

	resp, err := gw.SendSMS(context.Background(), SMSRequest{
		ReferenceID: "ref-5",
		To:          "15551234567",
		Message:     "hello",
	})
	if !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("expected ErrInvalidRequest, got %v", err)
	}
	if resp.ReferenceID != "ref-5" {
		t.Fatalf("expected referenceId ref-5, got %q", resp.ReferenceID)
	}
	if resp.Reason != "duplicate_reference" {
		t.Fatalf("expected duplicate_reference, got %q", resp.Reason)
	}
}

func TestSendSMSValidRequestAccepted(t *testing.T) {
	gw, err := New(Config{Provider: "24X7"})
	if err != nil {
		t.Fatalf("new gateway: %v", err)
	}

	resp, err := gw.SendSMS(context.Background(), SMSRequest{
		ReferenceID: "ref-6",
		To:          "15551234567",
		Message:     "hello",
	})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if resp.ReferenceID != "ref-6" {
		t.Fatalf("expected referenceId ref-6, got %q", resp.ReferenceID)
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
		ReferenceID: "ref-7",
		To:          "15551234567",
		Message:     "hello",
	})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if resp.ReferenceID != "ref-7" {
		t.Fatalf("expected referenceId ref-7, got %q", resp.ReferenceID)
	}
	if resp.Status != "rejected" {
		t.Fatalf("expected rejected status, got %q", resp.Status)
	}
	if resp.Reason != "provider_failure" {
		t.Fatalf("expected provider_failure, got %q", resp.Reason)
	}
}
