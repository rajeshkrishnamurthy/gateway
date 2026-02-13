package adapter

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"gateway"
	"gateway/pii"
	"net"
	"net/http"
	"time"
)

// SmsInfoBipProviderName is the identifier for the smsinfobip SMS provider adapter.
const SmsInfoBipProviderName = "smsinfobip-provider"

type infoBipRequestBody struct {
	Messages []infoBipMessage `json:"messages"`
}

type infoBipMessage struct {
	From         string               `json:"from"`
	Destinations []infoBipDestination `json:"destinations"`
	Text         string               `json:"text"`
}

type infoBipDestination struct {
	To string `json:"to"`
}

// SmsInfoBipProviderCall builds the ProviderCall for the InfoBip SMS provider.
// SmsInfoBipProviderCall builds a ProviderCall for the smsinfobip SMS provider.
func SmsInfoBipProviderCall(providerURL, apiKey, senderID string, connectTimeout time.Duration) gateway.ProviderCall {
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
		logger := providerLogger(SmsInfoBipProviderName, req.ReferenceID)

		requestBody := infoBipRequestBody{
			Messages: []infoBipMessage{
				{
					From: senderID,
					Destinations: []infoBipDestination{
						{To: req.To},
					},
					Text: req.Message,
				},
			},
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
		httpReq.Header.Set("App", apiKey)

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
		if resp.StatusCode >= http.StatusOK && resp.StatusCode < http.StatusMultipleChoices {
			logger.Info("sms provider decision", "event", "provider.decision", "outcome", "accepted", "status", resp.StatusCode, "mapped", "accepted")
			return gateway.ProviderResult{Status: "accepted"}, nil
		}
		logger.Info("sms provider decision", "event", "provider.decision", "outcome", "provider_failure", "status", resp.StatusCode, "mapped", "provider_failure")
		return gateway.ProviderResult{}, errors.New("provider non-2xx response")
	}
}
