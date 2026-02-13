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

// Sms24X7ProviderName is the identifier for the sms24x7 SMS provider adapter.
const Sms24X7ProviderName = "sms24x7-provider"

// Sms24X7ProviderCall builds the ProviderCall for the 24X7 SMS provider.
// Sms24X7ProviderCall builds a ProviderCall for the sms24x7 SMS provider.
func Sms24X7ProviderCall(providerURL, apiKey, serviceName, senderID string, connectTimeout time.Duration) gateway.ProviderCall {
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
		logger := providerLogger(Sms24X7ProviderName, req.ReferenceID)

		encodedRecipient := url.QueryEscape(req.To)
		encodedMessage := url.QueryEscape(req.Message)

		// url syntax requires ?. This has to be managed with providerUrl already containing ? etc.
		separator := "?"
		if strings.Contains(providerURL, "?") {
			if strings.HasSuffix(providerURL, "?") || strings.HasSuffix(providerURL, "&") {
				separator = ""
			} else {
				separator = "&"
			}
		}
		requestURL := providerURL + separator +
			"ApiKey=" + apiKey +
			"&ServiceName=" + serviceName +
			"&MobileNo=" + encodedRecipient +
			"&Message=" + encodedMessage +
			"&SenderId=" + senderID

		httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, requestURL, nil)
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

		// We are looking for status codes in the 2xx range for success.
		if resp.StatusCode >= http.StatusOK && resp.StatusCode < http.StatusMultipleChoices { // http.StatusMultipleChoices is 300 status code.
			logger.Info("sms provider decision", "event", "provider.decision", "outcome", "accepted", "status", resp.StatusCode, "mapped", "accepted")
			return gateway.ProviderResult{Status: "accepted"}, nil
		}
		logger.Info("sms provider decision", "event", "provider.decision", "outcome", "provider_failure", "status", resp.StatusCode, "mapped", "provider_failure")
		return gateway.ProviderResult{}, errors.New("provider non-2xx response")
	}
}
