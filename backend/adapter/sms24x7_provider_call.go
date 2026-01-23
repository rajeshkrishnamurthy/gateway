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

const Sms24X7ProviderName = "sms24x7-provider"

// Sms24X7ProviderCall builds the ProviderCall for the 24X7 SMS provider.
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
			log.Printf("sms provider error referenceId=%q provider=%q error=%q", req.ReferenceID, Sms24X7ProviderName, "request_build_failed")
			return gateway.ProviderResult{}, err
		}

		log.Printf(
			"sms provider request referenceId=%q provider=%q url=%q recipientMasked=%q messageLen=%d messageHash=%q",
			req.ReferenceID,
			Sms24X7ProviderName,
			providerURL,
			recipientMasked,
			messageLen,
			messageHash,
		)
		resp, err := client.Do(httpReq)
		if err != nil {
			log.Printf("sms provider error referenceId=%q provider=%q error=%q", req.ReferenceID, Sms24X7ProviderName, "request_failed")
			return gateway.ProviderResult{}, err
		}
		defer resp.Body.Close()

		log.Printf("sms provider response referenceId=%q provider=%q status=%d", req.ReferenceID, Sms24X7ProviderName, resp.StatusCode)

		// We are looking for status codes in the 2xx range for success.
		if resp.StatusCode >= http.StatusOK && resp.StatusCode < http.StatusMultipleChoices { // http.StatusMultipleChoices is 300 status code. 
			log.Printf("sms provider decision referenceId=%q provider=%q mapped=accepted", req.ReferenceID, Sms24X7ProviderName)
			return gateway.ProviderResult{Status: "accepted"}, nil
		}
		log.Printf("sms provider decision referenceId=%q provider=%q status=%d mapped=provider_failure", req.ReferenceID, Sms24X7ProviderName, resp.StatusCode)
		return gateway.ProviderResult{}, errors.New("provider non-2xx response")
	}
}
