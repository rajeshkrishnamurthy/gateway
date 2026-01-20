package gateway

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"strings"
)

var ErrInvalidRequest = errors.New("invalid request")
var errMissingProvider = errors.New("provider is required")
var errUnknownProvider = errors.New("provider is not supported")

const provider24x7 = "24x7"

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
type Gateway struct{}

// New constructs a Gateway instance.
func New(cfg Config) (*Gateway, error) {
	if cfg.Provider == "" {
		return nil, errMissingProvider
	}
	if !strings.EqualFold(cfg.Provider, provider24x7) {
		return nil, errUnknownProvider
	}
	return &Gateway{}, nil
}

// SendSMS submits an SMS request to the configured provider.
func (g *Gateway) SendSMS(ctx context.Context, req SMSRequest) (SMSResponse, error) {
	if req.ReferenceID == "" {
		return SMSResponse{
			Status: "rejected",
			Reason: "missing_reference_id",
		}, ErrInvalidRequest
	}
	if req.To == "" {
		return SMSResponse{
			ReferenceID: req.ReferenceID,
			Status:      "rejected",
			Reason:      "missing_to",
		}, ErrInvalidRequest
	}
	if req.Message == "" {
		return SMSResponse{
			ReferenceID: req.ReferenceID,
			Status:      "rejected",
			Reason:      "missing_message",
		}, ErrInvalidRequest
	}

	if ctx.Err() != nil {
		return SMSResponse{
			ReferenceID: req.ReferenceID,
			Status:      "rejected",
			Reason:      "gateway_unavailable",
		}, nil
	}

	messageID, err := newMessageID()
	if err != nil {
		return SMSResponse{}, err
	}
	return SMSResponse{
		ReferenceID:      req.ReferenceID,
		Status:           "accepted",
		GatewayMessageID: messageID,
	}, nil
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
