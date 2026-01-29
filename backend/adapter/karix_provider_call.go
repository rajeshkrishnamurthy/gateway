package adapter

import (
	"context"
	"errors"
	"gateway"
	"gateway/pii"
	"log"
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
			log.Printf("sms provider error referenceId=%q provider=%q error=%q", req.ReferenceID, SmsKarixProviderName, "request_build_failed")
			return gateway.ProviderResult{}, err
		}

		log.Printf(
			"sms provider request referenceId=%q provider=%q url=%q recipientMasked=%q messageLen=%d messageHash=%q",
			req.ReferenceID,
			SmsKarixProviderName,
			providerURL,
			recipientMasked,
			messageLen,
			messageHash,
		)
		resp, err := client.Do(httpReq)
		if err != nil {
			log.Printf("sms provider error referenceId=%q provider=%q error=%q", req.ReferenceID, SmsKarixProviderName, "request_failed")
			return gateway.ProviderResult{}, err
		}
		defer resp.Body.Close()

		log.Printf("sms provider response referenceId=%q provider=%q status=%d", req.ReferenceID, SmsKarixProviderName, resp.StatusCode)
		if resp.StatusCode >= http.StatusOK && resp.StatusCode < http.StatusMultipleChoices {
			log.Printf("sms provider decision referenceId=%q provider=%q mapped=accepted", req.ReferenceID, SmsKarixProviderName)
			return gateway.ProviderResult{Status: "accepted"}, nil
		}
		log.Printf("sms provider decision referenceId=%q provider=%q status=%d mapped=provider_failure", req.ReferenceID, SmsKarixProviderName, resp.StatusCode)
		return gateway.ProviderResult{}, errors.New("provider non-2xx response")
	}
}
