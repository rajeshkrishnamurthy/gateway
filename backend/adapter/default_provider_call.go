package adapter

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"gateway"
	"io"
	"log"
	"net"
	"net/http"
	"time"
)

const defaultProviderName = "default-provider"

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
		messageHash := hashMessage(req.Message)

		payload := providerRequest{
			ReferenceID: req.ReferenceID,
			To:          req.To,
			Message:     req.Message,
			TenantID:    req.TenantID,
		}
		body, err := json.Marshal(payload)
		if err != nil {
			log.Printf("sms provider error referenceId=%q provider=%q error=%v", req.ReferenceID, defaultProviderName, err)
			return gateway.ProviderResult{}, err
		}

		httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, providerURL, bytes.NewReader(body))
		if err != nil {
			log.Printf("sms provider error referenceId=%q provider=%q error=%v", req.ReferenceID, defaultProviderName, err)
			return gateway.ProviderResult{}, err
		}
		httpReq.Header.Set("Content-Type", "application/json")

		log.Printf(
			"sms provider request referenceId=%q provider=%q url=%q recipientMasked=%q messageLen=%d messageHash=%q",
			req.ReferenceID,
			defaultProviderName,
			providerURL,
			recipientMasked,
			messageLen,
			messageHash,
		)
		resp, err := client.Do(httpReq)
		if err != nil {
			log.Printf("sms provider error referenceId=%q provider=%q error=%v", req.ReferenceID, defaultProviderName, err)
			return gateway.ProviderResult{}, err
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			log.Printf("sms provider error referenceId=%q provider=%q status=%d", req.ReferenceID, defaultProviderName, resp.StatusCode)
			return gateway.ProviderResult{}, errors.New("provider non-200 response")
		}

		dec := json.NewDecoder(resp.Body)
		var providerResp providerResponse
		if err := dec.Decode(&providerResp); err != nil {
			log.Printf("sms provider error referenceId=%q provider=%q error=%v", req.ReferenceID, defaultProviderName, err)
			return gateway.ProviderResult{}, err
		}
		if err := dec.Decode(&struct{}{}); err != io.EOF {
			log.Printf("sms provider error referenceId=%q provider=%q error=%v", req.ReferenceID, defaultProviderName, err)
			return gateway.ProviderResult{}, errors.New("provider response has trailing data")
		}
		if providerResp.Status == "" {
			log.Printf("sms provider error referenceId=%q provider=%q error=%v", req.ReferenceID, defaultProviderName, "provider status missing")
			return gateway.ProviderResult{}, errors.New("provider status missing")
		}

		log.Printf(
			"sms provider response referenceId=%q provider=%q status=%q reason=%q",
			req.ReferenceID,
			defaultProviderName,
			providerResp.Status,
			providerResp.Reason,
		)
		return gateway.ProviderResult{
			Status: providerResp.Status,
			Reason: providerResp.Reason,
		}, nil
	}
}
