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
	"strings"
	"time"
)

const modelProviderName = "model-provider"

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
		messageHash := hashMessage(req.Message)

		requestBody := modelProviderRequestBody{
			Destination: req.To,
			Text:        req.Message,
		}
		body, err := json.Marshal(requestBody)
		if err != nil {
			log.Printf("sms provider error referenceId=%q provider=%q error=%v", req.ReferenceID, modelProviderName, err)
			return gateway.ProviderResult{}, err
		}

		httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, providerURL, bytes.NewReader(body))
		if err != nil {
			log.Printf("sms provider error referenceId=%q provider=%q error=%v", req.ReferenceID, modelProviderName, err)
			return gateway.ProviderResult{}, err
		}
		httpReq.Header.Set("Content-Type", "application/json")
		if req.ReferenceID != "" {
			httpReq.Header.Set("X-Request-Id", req.ReferenceID)
		}

		log.Printf(
			"sms provider request referenceId=%q provider=%q url=%q recipientMasked=%q messageLen=%d messageHash=%q",
			req.ReferenceID,
			modelProviderName,
			providerURL,
			recipientMasked,
			messageLen,
			messageHash,
		)
		resp, err := client.Do(httpReq)
		if err != nil {
			log.Printf("sms provider error referenceId=%q provider=%q error=%v", req.ReferenceID, modelProviderName, err)
			return gateway.ProviderResult{}, err
		}
		defer resp.Body.Close()

		log.Printf("sms provider response referenceId=%q provider=%q status=%d", req.ReferenceID, modelProviderName, resp.StatusCode)
		switch resp.StatusCode {
		case http.StatusOK:
			var successBody modelProviderSuccessBody
			dec := json.NewDecoder(resp.Body)
			if err := dec.Decode(&successBody); err != nil {
				log.Printf("sms provider error referenceId=%q provider=%q error=%v", req.ReferenceID, modelProviderName, err)
				return gateway.ProviderResult{}, err
			}
			if err := dec.Decode(&struct{}{}); err != io.EOF {
				log.Printf("sms provider error referenceId=%q provider=%q error=%v", req.ReferenceID, modelProviderName, err)
				return gateway.ProviderResult{}, errors.New("provider response has trailing data")
			}
			if successBody.Status != "OK" || strings.TrimSpace(successBody.ProviderID) == "" {
				log.Printf("sms provider error referenceId=%q provider=%q error=missing_required_fields", req.ReferenceID, modelProviderName)
				return gateway.ProviderResult{}, errors.New("provider response missing required fields")
			}
			log.Printf("sms provider decision referenceId=%q provider=%q mapped=accepted", req.ReferenceID, modelProviderName)
			return gateway.ProviderResult{Status: "accepted"}, nil
		case http.StatusBadRequest:
			var errorBody modelProviderErrorBody
			dec := json.NewDecoder(resp.Body)
			if err := dec.Decode(&errorBody); err != nil {
				log.Printf("sms provider error referenceId=%q provider=%q error=%v", req.ReferenceID, modelProviderName, err)
				return gateway.ProviderResult{}, err
			}
			if err := dec.Decode(&struct{}{}); err != io.EOF {
				log.Printf("sms provider error referenceId=%q provider=%q error=%v", req.ReferenceID, modelProviderName, err)
				return gateway.ProviderResult{}, errors.New("provider response has trailing data")
			}
			switch strings.TrimSpace(errorBody.Error) {
			case "INVALID_RECIPIENT":
				log.Printf("sms provider decision referenceId=%q provider=%q error=%q mapped=invalid_recipient", req.ReferenceID, modelProviderName, errorBody.Error)
				return gateway.ProviderResult{Status: "rejected", Reason: "invalid_recipient"}, nil
			case "INVALID_MESSAGE":
				log.Printf("sms provider decision referenceId=%q provider=%q error=%q mapped=invalid_message", req.ReferenceID, modelProviderName, errorBody.Error)
				return gateway.ProviderResult{Status: "rejected", Reason: "invalid_message"}, nil
			default:
				log.Printf("sms provider error referenceId=%q provider=%q error=%q", req.ReferenceID, modelProviderName, errorBody.Error)
				return gateway.ProviderResult{}, errors.New("provider response unknown error")
			}
		case http.StatusInternalServerError:
			log.Printf("sms provider decision referenceId=%q provider=%q status=%d mapped=provider_failure", req.ReferenceID, modelProviderName, resp.StatusCode)
			return gateway.ProviderResult{}, errors.New("provider failure")
		default:
			log.Printf("sms provider decision referenceId=%q provider=%q status=%d mapped=provider_failure", req.ReferenceID, modelProviderName, resp.StatusCode)
			return gateway.ProviderResult{}, errors.New("provider unexpected status")
		}
	}
}
