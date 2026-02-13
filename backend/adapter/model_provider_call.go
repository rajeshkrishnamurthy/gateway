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
	"strings"
	"time"
)

// ModelProviderName is the identifier for the model SMS provider adapter.
const ModelProviderName = "model-provider"

type modelProviderRequestBody struct {
	Destination string `json:"destination"`
	Text        string `json:"text"`
}

type modelProviderSuccessBody struct {
	Status     string `json:"status"`
	ProviderID string `json:"provider_id"`
}

type modelProviderErrorBody struct {
	Error string `json:"error"`
}

// ModelProviderCall builds the ProviderCall for the canonical model provider.
// ModelProviderCall builds a ProviderCall for the model SMS provider.
func ModelProviderCall(providerURL string, connectTimeout time.Duration) gateway.ProviderCall {
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
		logger := providerLogger(ModelProviderName, req.ReferenceID)

		requestBody := modelProviderRequestBody{
			Destination: req.To,
			Text:        req.Message,
		}
		body, err := json.Marshal(requestBody)
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
		if req.ReferenceID != "" {
			httpReq.Header.Set("X-Request-Id", req.ReferenceID)
		}

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

		logger.Info("sms provider response", "event", "provider.response", "outcome", "http_response", "status", resp.StatusCode)
		switch resp.StatusCode {
		case http.StatusOK:
			var successBody modelProviderSuccessBody
			dec := json.NewDecoder(resp.Body)
			if err := dec.Decode(&successBody); err != nil {
				logger.Error("sms provider success decode failed", "event", "provider.response.decode_failed", "outcome", "error", "status", resp.StatusCode, "error", err)
				return gateway.ProviderResult{}, err
			}
			if err := dec.Decode(&struct{}{}); err != io.EOF {
				logger.Error("sms provider success trailing data", "event", "provider.response.trailing_data", "outcome", "error", "status", resp.StatusCode, "error", err)
				return gateway.ProviderResult{}, errors.New("provider response has trailing data")
			}
			if successBody.Status != "OK" || strings.TrimSpace(successBody.ProviderID) == "" {
				logger.Error("sms provider success missing required fields", "event", "provider.response.invalid", "outcome", "error", "status", resp.StatusCode, "errorCode", "missing_required_fields")
				return gateway.ProviderResult{}, errors.New("provider response missing required fields")
			}
			logger.Info("sms provider decision", "event", "provider.decision", "outcome", "accepted", "mapped", "accepted", "status", resp.StatusCode)
			return gateway.ProviderResult{Status: "accepted"}, nil
		case http.StatusBadRequest:
			var errorBody modelProviderErrorBody
			dec := json.NewDecoder(resp.Body)
			if err := dec.Decode(&errorBody); err != nil {
				logger.Error("sms provider error decode failed", "event", "provider.response.decode_failed", "outcome", "error", "status", resp.StatusCode, "error", err)
				return gateway.ProviderResult{}, err
			}
			if err := dec.Decode(&struct{}{}); err != io.EOF {
				logger.Error("sms provider error trailing data", "event", "provider.response.trailing_data", "outcome", "error", "status", resp.StatusCode, "error", err)
				return gateway.ProviderResult{}, errors.New("provider response has trailing data")
			}
			switch strings.TrimSpace(errorBody.Error) {
			case "INVALID_RECIPIENT":
				logger.Info("sms provider decision", "event", "provider.decision", "outcome", "rejected", "status", resp.StatusCode, "errorCode", errorBody.Error, "mapped", "invalid_recipient")
				return gateway.ProviderResult{Status: "rejected", Reason: "invalid_recipient"}, nil
			case "INVALID_MESSAGE":
				logger.Info("sms provider decision", "event", "provider.decision", "outcome", "rejected", "status", resp.StatusCode, "errorCode", errorBody.Error, "mapped", "invalid_message")
				return gateway.ProviderResult{Status: "rejected", Reason: "invalid_message"}, nil
			default:
				logger.Error("sms provider unknown error mapping", "event", "provider.response.invalid", "outcome", "error", "status", resp.StatusCode, "errorCode", errorBody.Error)
				return gateway.ProviderResult{}, errors.New("provider response unknown error")
			}
		case http.StatusInternalServerError:
			logger.Info("sms provider decision", "event", "provider.decision", "outcome", "provider_failure", "status", resp.StatusCode, "mapped", "provider_failure")
			return gateway.ProviderResult{}, errors.New("provider failure")
		default:
			logger.Info("sms provider decision", "event", "provider.decision", "outcome", "provider_failure", "status", resp.StatusCode, "mapped", "provider_failure")
			return gateway.ProviderResult{}, errors.New("provider unexpected status")
		}
	}
}
