package adapter

import (
	"context"
	"errors"
	"gateway"
	"gateway/pii"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// SmsKarixProviderName is the identifier for the smskarix SMS provider adapter.
const SmsKarixProviderName = "smskarix-provider"

// SmsKarixProviderCall builds the ProviderCall for the Karix SMS provider.
// SmsKarixProviderCall builds a ProviderCall for the smskarix SMS provider.
func SmsKarixProviderCall(providerURL, apiKey, version, senderID string, connectTimeout time.Duration) gateway.ProviderCall {
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
		logger := providerLogger(SmsKarixProviderName, req.ReferenceID)

		separator := "?"
		if strings.Contains(providerURL, "?") {
			if strings.HasSuffix(providerURL, "?") || strings.HasSuffix(providerURL, "&") {
				separator = ""
			} else {
				separator = "&"
			}
		}
		encodedVersion := url.QueryEscape(version)
		encodedAPIKey := url.QueryEscape(apiKey)
		encodedRecipient := url.QueryEscape(req.To)
		encodedSenderID := url.QueryEscape(senderID)
		encodedMessage := url.QueryEscape(req.Message)
		requestURL := providerURL + separator +
			"ver=" + encodedVersion +
			"&key=" + encodedAPIKey +
			"&encrpt=0" +
			"&dest=" + encodedRecipient +
			"&send=" + encodedSenderID +
			"&text=" + encodedMessage

		httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
		if err != nil {
			logger.Error("sms provider request build failed", "event", "provider.request.build_failed", "outcome", "error", "errorCode", "request_build_failed")
			return gateway.ProviderResult{}, err
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
			logger.Error("sms provider request failed", "event", "provider.request.failed", "outcome", "error", "errorCode", "request_failed")
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
