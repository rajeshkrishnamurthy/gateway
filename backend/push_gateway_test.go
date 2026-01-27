package gateway

import (
	"context"
	"errors"
	"testing"
	"time"
)

const defaultPushProviderTimeout = 30 * time.Second

func TestNewPushMissingProvider(t *testing.T) {
	_, err := NewPushGateway(PushConfig{})
	if err == nil {
		t.Fatal("expected error for missing provider")
	}
}

func TestNewPushInvalidProviderTimeoutLow(t *testing.T) {
	_, err := NewPushGateway(PushConfig{
		ProviderCall: func(ctx context.Context, req PushRequest) (ProviderResult, error) {
			return ProviderResult{Status: "accepted"}, nil
		},
		ProviderTimeout: 10 * time.Second,
	})
	if err == nil {
		t.Fatal("expected error for invalid provider timeout")
	}
}

func TestNewPushInvalidProviderTimeoutHigh(t *testing.T) {
	_, err := NewPushGateway(PushConfig{
		ProviderCall: func(ctx context.Context, req PushRequest) (ProviderResult, error) {
			return ProviderResult{Status: "accepted"}, nil
		},
		ProviderTimeout: 61 * time.Second,
	})
	if err == nil {
		t.Fatal("expected error for invalid provider timeout")
	}
}

func TestSendPushMissingReferenceID(t *testing.T) {
	gw, err := NewPushGateway(PushConfig{
		ProviderCall: func(ctx context.Context, req PushRequest) (ProviderResult, error) {
			return ProviderResult{Status: "accepted"}, nil
		},
		ProviderTimeout: defaultPushProviderTimeout,
	})
	if err != nil {
		t.Fatalf("new gateway: %v", err)
	}

	resp, err := gw.SendPush(context.Background(), PushRequest{
		Token: "token-1",
		Body:  "hello",
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

func TestSendPushMissingToken(t *testing.T) {
	gw, err := NewPushGateway(PushConfig{
		ProviderCall: func(ctx context.Context, req PushRequest) (ProviderResult, error) {
			return ProviderResult{Status: "accepted"}, nil
		},
		ProviderTimeout: defaultPushProviderTimeout,
	})
	if err != nil {
		t.Fatalf("new gateway: %v", err)
	}

	resp, err := gw.SendPush(context.Background(), PushRequest{
		ReferenceID: "ref-1",
		Body:        "hello",
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

func TestSendPushMissingPayload(t *testing.T) {
	gw, err := NewPushGateway(PushConfig{
		ProviderCall: func(ctx context.Context, req PushRequest) (ProviderResult, error) {
			return ProviderResult{Status: "accepted"}, nil
		},
		ProviderTimeout: defaultPushProviderTimeout,
	})
	if err != nil {
		t.Fatalf("new gateway: %v", err)
	}

	resp, err := gw.SendPush(context.Background(), PushRequest{
		ReferenceID: "ref-2",
		Token:       "token-2",
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

func TestSendPushDataOnlyAllowed(t *testing.T) {
	gw, err := NewPushGateway(PushConfig{
		ProviderCall: func(ctx context.Context, req PushRequest) (ProviderResult, error) {
			return ProviderResult{Status: "accepted"}, nil
		},
		ProviderTimeout: defaultPushProviderTimeout,
	})
	if err != nil {
		t.Fatalf("new gateway: %v", err)
	}

	resp, err := gw.SendPush(context.Background(), PushRequest{
		ReferenceID: "ref-data",
		Token:       "token-3",
		Data: map[string]string{
			"key": "value",
		},
	})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if resp.Status != "accepted" {
		t.Fatalf("expected accepted status, got %q", resp.Status)
	}
	if resp.GatewayMessageID == "" {
		t.Fatalf("expected non-empty gatewayMessageId")
	}
}

func TestSendPushProviderInvalidRecipientMapsToInvalidRequest(t *testing.T) {
	gw, err := NewPushGateway(PushConfig{
		ProviderCall: func(ctx context.Context, req PushRequest) (ProviderResult, error) {
			return ProviderResult{Status: "rejected", Reason: "invalid_recipient"}, nil
		},
		ProviderTimeout: defaultPushProviderTimeout,
	})
	if err != nil {
		t.Fatalf("new gateway: %v", err)
	}

	resp, err := gw.SendPush(context.Background(), PushRequest{
		ReferenceID: "ref-3",
		Token:       "token-4",
		Body:        "hello",
	})
	if !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("expected ErrInvalidRequest, got %v", err)
	}
	if resp.ReferenceID != "ref-3" {
		t.Fatalf("expected referenceId ref-3, got %q", resp.ReferenceID)
	}
	if resp.Reason != "invalid_request" {
		t.Fatalf("expected invalid_request, got %q", resp.Reason)
	}
}

func TestSendPushProviderInvalidMessageMapsToInvalidRequest(t *testing.T) {
	gw, err := NewPushGateway(PushConfig{
		ProviderCall: func(ctx context.Context, req PushRequest) (ProviderResult, error) {
			return ProviderResult{Status: "rejected", Reason: "invalid_message"}, nil
		},
		ProviderTimeout: defaultPushProviderTimeout,
	})
	if err != nil {
		t.Fatalf("new gateway: %v", err)
	}

	resp, err := gw.SendPush(context.Background(), PushRequest{
		ReferenceID: "ref-4",
		Token:       "token-5",
		Body:        "hello",
	})
	if !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("expected ErrInvalidRequest, got %v", err)
	}
	if resp.ReferenceID != "ref-4" {
		t.Fatalf("expected referenceId ref-4, got %q", resp.ReferenceID)
	}
	if resp.Reason != "invalid_request" {
		t.Fatalf("expected invalid_request, got %q", resp.Reason)
	}
}

func TestSendPushProviderFailureResult(t *testing.T) {
	gw, err := NewPushGateway(PushConfig{
		ProviderCall: func(ctx context.Context, req PushRequest) (ProviderResult, error) {
			return ProviderResult{Status: "rejected", Reason: "provider_failure"}, nil
		},
		ProviderTimeout: defaultPushProviderTimeout,
	})
	if err != nil {
		t.Fatalf("new gateway: %v", err)
	}

	resp, err := gw.SendPush(context.Background(), PushRequest{
		ReferenceID: "ref-4b",
		Token:       "token-6",
		Body:        "hello",
	})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if resp.ReferenceID != "ref-4b" {
		t.Fatalf("expected referenceId ref-4b, got %q", resp.ReferenceID)
	}
	if resp.Reason != "provider_failure" {
		t.Fatalf("expected provider_failure, got %q", resp.Reason)
	}
}

func TestSendPushProviderUnregisteredToken(t *testing.T) {
	gw, err := NewPushGateway(PushConfig{
		ProviderCall: func(ctx context.Context, req PushRequest) (ProviderResult, error) {
			return ProviderResult{Status: "rejected", Reason: "unregistered_token"}, nil
		},
		ProviderTimeout: defaultPushProviderTimeout,
	})
	if err != nil {
		t.Fatalf("new gateway: %v", err)
	}

	resp, err := gw.SendPush(context.Background(), PushRequest{
		ReferenceID: "ref-unreg",
		Token:       "token-unreg",
		Body:        "hello",
	})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if resp.Status != "rejected" {
		t.Fatalf("expected rejected status, got %q", resp.Status)
	}
	if resp.Reason != "unregistered_token" {
		t.Fatalf("expected unregistered_token, got %q", resp.Reason)
	}
}

func TestSendPushProviderPanic(t *testing.T) {
	gw, err := NewPushGateway(PushConfig{
		ProviderCall: func(ctx context.Context, req PushRequest) (ProviderResult, error) {
			panic("boom")
		},
		ProviderTimeout: defaultPushProviderTimeout,
	})
	if err != nil {
		t.Fatalf("new gateway: %v", err)
	}

	resp, err := gw.SendPush(context.Background(), PushRequest{
		ReferenceID: "ref-panic-1",
		Token:       "token-7",
		Body:        "hello",
	})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if resp.ReferenceID != "ref-panic-1" {
		t.Fatalf("expected referenceId ref-panic-1, got %q", resp.ReferenceID)
	}
	if resp.Status != "rejected" {
		t.Fatalf("expected rejected status, got %q", resp.Status)
	}
	if resp.Reason != "provider_failure" {
		t.Fatalf("expected provider_failure, got %q", resp.Reason)
	}
}

func TestSendPushDuplicateReferenceInFlight(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	gw, err := NewPushGateway(PushConfig{
		ProviderCall: func(ctx context.Context, req PushRequest) (ProviderResult, error) {
			close(started)
			<-release
			return ProviderResult{Status: "accepted"}, nil
		},
		ProviderTimeout: defaultPushProviderTimeout,
	})
	if err != nil {
		t.Fatalf("new gateway: %v", err)
	}

	errCh := make(chan error, 1)
	go func() {
		_, err := gw.SendPush(context.Background(), PushRequest{
			ReferenceID: "ref-5",
			Token:       "token-8",
			Body:        "hello",
		})
		errCh <- err
	}()

	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("expected provider call to start")
	}

	resp, err := gw.SendPush(context.Background(), PushRequest{
		ReferenceID: "ref-5",
		Token:       "token-8",
		Body:        "hello",
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

	close(release)
	if err := <-errCh; err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
}

func TestSendPushValidRequestAccepted(t *testing.T) {
	gw, err := NewPushGateway(PushConfig{
		ProviderCall: func(ctx context.Context, req PushRequest) (ProviderResult, error) {
			return ProviderResult{Status: "accepted"}, nil
		},
		ProviderTimeout: defaultPushProviderTimeout,
	})
	if err != nil {
		t.Fatalf("new gateway: %v", err)
	}

	resp, err := gw.SendPush(context.Background(), PushRequest{
		ReferenceID: "ref-6",
		Token:       "token-9",
		Body:        "hello",
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

func TestSendPushContextCanceled(t *testing.T) {
	gw, err := NewPushGateway(PushConfig{
		ProviderCall: func(ctx context.Context, req PushRequest) (ProviderResult, error) {
			return ProviderResult{}, ctx.Err()
		},
		ProviderTimeout: defaultPushProviderTimeout,
	})
	if err != nil {
		t.Fatalf("new gateway: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	resp, err := gw.SendPush(ctx, PushRequest{
		ReferenceID: "ref-7",
		Token:       "token-10",
		Body:        "hello",
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

func TestSendPushProviderTimeoutApplied(t *testing.T) {
	var remaining time.Duration
	gw, err := NewPushGateway(PushConfig{
		ProviderCall: func(ctx context.Context, req PushRequest) (ProviderResult, error) {
			deadline, ok := ctx.Deadline()
			if !ok {
				t.Fatal("expected provider deadline")
			}
			remaining = time.Until(deadline)
			return ProviderResult{Status: "accepted"}, nil
		},
		ProviderTimeout: 15 * time.Second,
	})
	if err != nil {
		t.Fatalf("new gateway: %v", err)
	}

	_, err = gw.SendPush(context.Background(), PushRequest{
		ReferenceID: "ref-timeout-1",
		Token:       "token-11",
		Body:        "hello",
	})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if remaining <= 0 {
		t.Fatalf("expected positive deadline remaining, got %v", remaining)
	}
	if remaining > 15*time.Second {
		t.Fatalf("expected deadline <= 15s, got %v", remaining)
	}
}
