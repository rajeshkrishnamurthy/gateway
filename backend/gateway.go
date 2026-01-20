package gateway

import (
	"context"
	"errors"
)

var errNotImplemented = errors.New("not implemented")
var errMissingProvider = errors.New("provider is required")

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

// Config defines Gateway configuration.
type Config struct {
	Provider string
}

// Gateway is the core SMS gateway service.
type Gateway struct {
	provider string
}

// New constructs a Gateway instance.
func New(cfg Config) (*Gateway, error) {
	if cfg.Provider == "" {
		return nil, errMissingProvider
	}
	return &Gateway{provider: cfg.Provider}, nil
}

// SendSMS submits an SMS request to the configured provider.
func (g *Gateway) SendSMS(ctx context.Context, req SMSRequest) (SMSResponse, error) {
	return SMSResponse{}, errNotImplemented
}
