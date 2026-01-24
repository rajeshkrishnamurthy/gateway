package gateway

import (
	"context"
	"errors"
	"gateway/metrics"
	"log"
	"sync"
	"time"
)

// PushRequest is the domain input for submitting a push send request.
type PushRequest struct {
	ReferenceID string            `json:"referenceId"`
	Token       string            `json:"token"`
	Title       string            `json:"title,omitempty"`
	Body        string            `json:"body,omitempty"`
	Data        map[string]string `json:"data,omitempty"`
	TenantID    string            `json:"tenantId,omitempty"`
}

// PushResponse is the domain output for a push send attempt.
type PushResponse struct {
	ReferenceID      string `json:"referenceId"`
	Status           string `json:"status"`
	GatewayMessageID string `json:"gatewayMessageId,omitempty"`
	Reason           string `json:"reason,omitempty"`
}

// PushConfig defines push gateway configuration.
type PushConfig struct {
	ProviderCall    PushProviderCall
	ProviderTimeout time.Duration
	Metrics         *metrics.Registry
}

// PushProviderCall invokes the configured push provider.
type PushProviderCall func(context.Context, PushRequest) (ProviderResult, error)

// PushGateway is the core push gateway service.
type PushGateway struct {
	mu sync.Mutex
	// Gateway is submission-only with no durable state, so idempotency is limited to in-flight requests.
	inflight        map[string]struct{}
	providerCall    PushProviderCall
	providerTimeout time.Duration
	metrics         *metrics.Registry
}

// NewPushGateway constructs a PushGateway instance.
func NewPushGateway(cfg PushConfig) (*PushGateway, error) {
	if cfg.ProviderCall == nil {
		return nil, errMissingProviderCall
	}
	if cfg.ProviderTimeout < minProviderTimeout || cfg.ProviderTimeout > maxProviderTimeout {
		return nil, errInvalidProviderTimeout
	}
	return &PushGateway{
		inflight:        make(map[string]struct{}),
		providerCall:    cfg.ProviderCall,
		providerTimeout: cfg.ProviderTimeout,
		metrics:         cfg.Metrics,
	}, nil
}

// SendPush submits a push request to the configured provider.
func (g *PushGateway) SendPush(ctx context.Context, req PushRequest) (PushResponse, error) {
	if req.ReferenceID == "" {
		status := "rejected"
		reason := "invalid_request"
		return PushResponse{
			ReferenceID: req.ReferenceID,
			Status:      status,
			Reason:      reason,
		}, ErrInvalidRequest
	}
	if req.Token == "" {
		status := "rejected"
		reason := "invalid_request"
		return PushResponse{
			ReferenceID: req.ReferenceID,
			Status:      status,
			Reason:      reason,
		}, ErrInvalidRequest
	}
	if req.Title == "" && req.Body == "" && len(req.Data) == 0 {
		status := "rejected"
		reason := "invalid_request"
		return PushResponse{
			ReferenceID: req.ReferenceID,
			Status:      status,
			Reason:      reason,
		}, ErrInvalidRequest
	}

	g.mu.Lock()
	if _, ok := g.inflight[req.ReferenceID]; ok {
		g.mu.Unlock()
		status := "rejected"
		reason := "duplicate_reference"
		return PushResponse{
			ReferenceID: req.ReferenceID,
			Status:      status,
			Reason:      reason,
		}, ErrInvalidRequest
	}
	g.inflight[req.ReferenceID] = struct{}{}
	g.mu.Unlock()
	defer func() {
		g.mu.Lock()
		delete(g.inflight, req.ReferenceID)
		g.mu.Unlock()
	}()

	providerCtx, cancel := context.WithTimeout(ctx, g.providerTimeout)
	defer cancel()

	providerStart := time.Now()
	panicRecovered := false
	providerResult, err := func() (providerResult ProviderResult, err error) {
		// Normalize provider panics to provider_failure to keep the gateway contract stable.
		defer func() {
			if r := recover(); r != nil {
				panicRecovered = true
				log.Printf("push provider panic referenceId=%q panic=%v", req.ReferenceID, r)
				err = errors.New("provider panic")
			}
		}()
		return g.providerCall(providerCtx, req)
	}()
	if g.metrics != nil {
		g.metrics.ObserveProviderCall(time.Since(providerStart), err, panicRecovered)
	}
	if err != nil {
		status := "rejected"
		reason := "provider_failure"
		return PushResponse{
			ReferenceID: req.ReferenceID,
			Status:      status,
			Reason:      reason,
		}, nil
	}
	switch providerResult.Status {
	case "accepted":
		messageID, err := newMessageID()
		if err != nil {
			status := "rejected"
			reason := "provider_failure"
			return PushResponse{
				ReferenceID: req.ReferenceID,
				Status:      status,
				Reason:      reason,
			}, nil
		}
		status := "accepted"
		return PushResponse{
			ReferenceID:      req.ReferenceID,
			Status:           status,
			GatewayMessageID: messageID,
		}, nil
	case "rejected":
		if providerResult.Reason == "provider_failure" {
			status := "rejected"
			reason := "provider_failure"
			return PushResponse{
				ReferenceID: req.ReferenceID,
				Status:      status,
				Reason:      reason,
			}, nil
		}
		status := "rejected"
		reason := "invalid_request"
		return PushResponse{
			ReferenceID: req.ReferenceID,
			Status:      status,
			Reason:      reason,
		}, ErrInvalidRequest
	default:
		status := "rejected"
		reason := "provider_failure"
		return PushResponse{
			ReferenceID: req.ReferenceID,
			Status:      status,
			Reason:      reason,
		}, nil
	}
}
