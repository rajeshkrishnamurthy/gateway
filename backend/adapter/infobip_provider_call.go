package adapter

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"gateway"
	"gateway/pii"
	"log"
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
			log.Printf("sms provider error referenceId=%q provider=%q error=%v", req.ReferenceID, SmsInfoBipProviderName, err)
			return gateway.ProviderResult{}, err
		}

		httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, providerURL, bytes.NewReader(body))
		if err != nil {
			log.Printf("sms provider error referenceId=%q provider=%q error=%v", req.ReferenceID, SmsInfoBipProviderName, err)
			return gateway.ProviderResult{}, err
		}
		httpReq.Header.Set("Content-Type", "application/json")
		httpReq.Header.Set("App", apiKey)

		log.Printf(
			"sms provider request referenceId=%q provider=%q url=%q recipientMasked=%q messageLen=%d messageHash=%q",
			req.ReferenceID,
			SmsInfoBipProviderName,
			providerURL,
			recipientMasked,
			messageLen,
			messageHash,
		)
		resp, err := client.Do(httpReq)
		if err != nil {
			log.Printf("sms provider error referenceId=%q provider=%q error=%v", req.ReferenceID, SmsInfoBipProviderName, err)
			return gateway.ProviderResult{}, err
		}
		defer resp.Body.Close()

		log.Printf("sms provider response referenceId=%q provider=%q status=%d", req.ReferenceID, SmsInfoBipProviderName, resp.StatusCode)
		if resp.StatusCode >= http.StatusOK && resp.StatusCode < http.StatusMultipleChoices {
			log.Printf("sms provider decision referenceId=%q provider=%q mapped=accepted", req.ReferenceID, SmsInfoBipProviderName)
			return gateway.ProviderResult{Status: "accepted"}, nil
		}
		log.Printf("sms provider decision referenceId=%q provider=%q status=%d mapped=provider_failure", req.ReferenceID, SmsInfoBipProviderName, resp.StatusCode)
		return gateway.ProviderResult{}, errors.New("provider non-2xx response")
	}
}
