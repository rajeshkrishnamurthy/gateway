package adapter

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"gateway"
	"gateway/pii"
	"io"
	"net"
	"net/http"
	"time"
)

// DefaultProviderName is the identifier for the default SMS provider adapter.
const DefaultProviderName = "default-provider"

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

// DefaultProviderCall builds the ProviderCall for the default SMS provider protocol.
func DefaultProviderCall(providerURL string, connectTimeout time.Duration) gateway.ProviderCall {
	if providerURL == "" {
		return nil
	}

	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.DialContext = (&net.Dialer{
		Timeout:   connectTimeout,
		KeepAlive: 30 * time.Second,
	}).DialContext

	client := &http.Client{
		Transport: transport,
	}
	return func(ctx context.Context, req gateway.SMSRequest) (gateway.ProviderResult, error) {
		recipientMasked := maskRecipient(req.To)
		messageLen := len(req.Message)
		messageHash := pii.Hash(req.Message)
		logger := providerLogger(DefaultProviderName, req.ReferenceID)

		payload := providerRequest{
			ReferenceID: req.ReferenceID,
			To:          req.To,
			Message:     req.Message,
			TenantID:    req.TenantID,
		}
		body, err := json.Marshal(payload)
		if err != nil {
			logger.Error("sms provider request marshal failed", "event", "provider.request.encode_failed", "outcome", "error", "error", err)
			return gateway.ProviderResult{}, err
		}

		httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, providerURL, bytes.NewReader(body))
		if err != nil {
			logger.Error("sms provider request build failed", "event", "provider.request.build_failed", "outcome", "error", "error", err)
			return gateway.ProviderResult{}, err
		}
		httpReq.Header.Set("Content-Type", "application/json")

		logger.Info(
			"sms provider request",
			"event", "provider.request",
			"outcome", "attempt",
			"url", providerURL,
			"recipientMasked", recipientMasked,
			"messageLen", messageLen,
			"messageHash", messageHash,
		)
		resp, err := client.Do(httpReq)
		if err != nil {
			logger.Error("sms provider request failed", "event", "provider.request.failed", "outcome", "error", "error", err)
			return gateway.ProviderResult{}, err
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			logger.Error("sms provider non-200 response", "event", "provider.response.invalid_status", "outcome", "provider_failure", "status", resp.StatusCode)
			return gateway.ProviderResult{}, errors.New("provider non-200 response")
		}

		dec := json.NewDecoder(resp.Body)
		var providerResp providerResponse
		if err := dec.Decode(&providerResp); err != nil {
			logger.Error("sms provider response decode failed", "event", "provider.response.decode_failed", "outcome", "error", "status", resp.StatusCode, "error", err)
			return gateway.ProviderResult{}, err
		}
		if err := dec.Decode(&struct{}{}); err != io.EOF {
			logger.Error("sms provider response trailing data", "event", "provider.response.trailing_data", "outcome", "error", "status", resp.StatusCode, "error", err)
			return gateway.ProviderResult{}, errors.New("provider response has trailing data")
		}
		if providerResp.Status == "" {
			logger.Error("sms provider response missing status", "event", "provider.response.invalid", "outcome", "error", "status", resp.StatusCode, "errorCode", "provider_status_missing")
			return gateway.ProviderResult{}, errors.New("provider status missing")
		}

		logger.Info(
			"sms provider response",
			"event", "provider.response",
			"outcome", providerResp.Status,
			"status", resp.StatusCode,
			"providerStatus", providerResp.Status,
			"providerReason", providerResp.Reason,
		)
		return gateway.ProviderResult{
			Status: providerResp.Status,
			Reason: providerResp.Reason,
		}, nil
	}
}
