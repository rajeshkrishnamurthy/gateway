package gateway

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"sync"
	"time"
)

var ErrInvalidRequest = errors.New("invalid request")
var errMissingProvider = errors.New("provider is required")
var errInvalidProviderTimeout = errors.New("provider timeout must be between 15s and 60s")

const (
	minProviderTimeout = 15 * time.Second
	maxProviderTimeout = 60 * time.Second
)

// SMSRequest is the domain input for submitting an SMS send request.
type SMSRequest struct {
	ReferenceID string `json:"referenceId"`
	To          string `json:"to"`
	Message     string `json:"message"`
	TenantID    string `json:"tenantId,omitempty"`
}

// SMSResponse is the domain output for an SMS send attempt.
type SMSResponse struct {
	ReferenceID      string `json:"referenceId"`
	Status           string `json:"status"`
	GatewayMessageID string `json:"gatewayMessageId,omitempty"`
	Reason           string `json:"reason,omitempty"`
}

// Config defines SMS gateway configuration.
type Config struct {
	Provider        ProviderCall
	ProviderTimeout time.Duration
}

// ProviderCall invokes the configured SMS provider.
type ProviderCall func(context.Context, SMSRequest) (ProviderResult, error)

// ProviderResult describes the provider outcome for a request.
type ProviderResult struct {
	Status string
	Reason string
}

// SMSGateway is the core SMS gateway service.
type SMSGateway struct {
	mu              sync.Mutex
	seen            map[string]struct{}
	provider        ProviderCall
	providerTimeout time.Duration
}

// New constructs an SMSGateway instance.
func New(cfg Config) (*SMSGateway, error) {
	if cfg.Provider == nil {
		return nil, errMissingProvider
	}
	if cfg.ProviderTimeout < minProviderTimeout || cfg.ProviderTimeout > maxProviderTimeout {
		return nil, errInvalidProviderTimeout
	}
	return &SMSGateway{
		seen:            make(map[string]struct{}),
		provider:        cfg.Provider,
		providerTimeout: cfg.ProviderTimeout,
	}, nil
}

// SendSMS submits an SMS request to the configured provider.
func (g *SMSGateway) SendSMS(ctx context.Context, req SMSRequest) (SMSResponse, error) {
	if req.ReferenceID == "" {
		status := "rejected"
		reason := "invalid_request"
		return SMSResponse{
			ReferenceID: req.ReferenceID,
			Status:      status,
			Reason:      reason,
		}, ErrInvalidRequest
	}
	if req.To == "" {
		status := "rejected"
		reason := "invalid_request"
		return SMSResponse{
			ReferenceID: req.ReferenceID,
			Status:      status,
			Reason:      reason,
		}, ErrInvalidRequest
	}
	if req.Message == "" {
		status := "rejected"
		reason := "invalid_request"
		return SMSResponse{
			ReferenceID: req.ReferenceID,
			Status:      status,
			Reason:      reason,
		}, ErrInvalidRequest
	}

	g.mu.Lock()
	if _, ok := g.seen[req.ReferenceID]; ok {
		g.mu.Unlock()
		status := "rejected"
		reason := "duplicate_reference"
		return SMSResponse{
			ReferenceID: req.ReferenceID,
			Status:      status,
			Reason:      reason,
		}, ErrInvalidRequest
	}
	// referenceId is consumed on first sight to keep idempotency strict.
	// Invalid or failed attempts also consume the id in Phase 3.
	g.seen[req.ReferenceID] = struct{}{}
	g.mu.Unlock()

	providerCtx, cancel := context.WithTimeout(ctx, g.providerTimeout)
	defer cancel()

	providerResult, err := g.provider(providerCtx, req)
	if err != nil {
		status := "rejected"
		reason := "provider_failure"
		return SMSResponse{
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
			return SMSResponse{
				ReferenceID: req.ReferenceID,
				Status:      status,
				Reason:      reason,
			}, nil
		}
		status := "accepted"
		return SMSResponse{
			ReferenceID:      req.ReferenceID,
			Status:           status,
			GatewayMessageID: messageID,
		}, nil
	case "rejected":
		switch providerResult.Reason {
		case "invalid_recipient", "invalid_message":
			status := "rejected"
			reason := providerResult.Reason
			return SMSResponse{
				ReferenceID: req.ReferenceID,
				Status:      status,
				Reason:      reason,
			}, ErrInvalidRequest
		case "provider_failure":
			status := "rejected"
			reason := "provider_failure"
			return SMSResponse{
				ReferenceID: req.ReferenceID,
				Status:      status,
				Reason:      reason,
			}, nil
		default:
			status := "rejected"
			reason := "provider_failure"
			return SMSResponse{
				ReferenceID: req.ReferenceID,
				Status:      status,
				Reason:      reason,
			}, nil
		}
	default:
		status := "rejected"
		reason := "provider_failure"
		return SMSResponse{
			ReferenceID: req.ReferenceID,
			Status:      status,
			Reason:      reason,
		}, nil
	}
}

func newMessageID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return formatUUID(b), nil
}

func formatUUID(b [16]byte) string {
	var buf [36]byte
	hex.Encode(buf[0:8], b[0:4])
	buf[8] = '-'
	hex.Encode(buf[9:13], b[4:6])
	buf[13] = '-'
	hex.Encode(buf[14:18], b[6:8])
	buf[18] = '-'
	hex.Encode(buf[19:23], b[8:10])
	buf[23] = '-'
	hex.Encode(buf[24:36], b[10:16])
	return string(buf[:])
}
