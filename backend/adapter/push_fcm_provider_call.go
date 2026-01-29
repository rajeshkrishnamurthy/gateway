package adapter

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"gateway"
	"gateway/pii"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"
)

// PushFCMProviderName is the identifier for the FCM push provider adapter.
const PushFCMProviderName = "pushfcm-provider"
const fcmDebugEnv = "PUSH_FCM_DEBUG"
const fcmDebugMaxBytes = 2048

type fcmRequestBody struct {
	Message fcmMessage `json:"message"`
}

type fcmMessage struct {
	Token        string            `json:"token"`
	Notification *fcmNotification  `json:"notification,omitempty"`
	Data         map[string]string `json:"data,omitempty"`
}

type fcmNotification struct {
	Title string `json:"title,omitempty"`
	Body  string `json:"body,omitempty"`
}

type fcmErrorResponse struct {
	Error fcmError `json:"error"`
}

type fcmError struct {
	Details []fcmErrorDetail `json:"details"`
}

type fcmErrorDetail struct {
	ErrorCode string `json:"errorCode"`
}

// PushFCMProviderCall builds the ProviderCall for the FCM push provider.
// PushFCMProviderCall builds a PushProviderCall using a static bearer token.
func PushFCMProviderCall(providerURL, bearerToken string, connectTimeout time.Duration) gateway.PushProviderCall {
	if providerURL == "" {
		return nil
	}
	return PushFCMProviderCallWithTokenSource(providerURL, func(context.Context) (string, error) {
		return bearerToken, nil
	}, connectTimeout)
}

// PushFCMProviderCallWithTokenSource builds the ProviderCall for the FCM push provider.
// PushFCMProviderCallWithTokenSource builds a PushProviderCall using a token source callback.
func PushFCMProviderCallWithTokenSource(providerURL string, tokenSource func(context.Context) (string, error), connectTimeout time.Duration) gateway.PushProviderCall {
	if providerURL == "" || tokenSource == nil {
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
	return func(ctx context.Context, req gateway.PushRequest) (gateway.ProviderResult, error) {
		token, err := tokenSource(ctx)
		if err != nil {
			log.Printf("push provider error referenceId=%q provider=%q error=%v", req.ReferenceID, PushFCMProviderName, err)
			return gateway.ProviderResult{}, err
		}

		tokenMasked := maskRecipient(req.Token)
		payloadText := pushPayloadSummary(req)
		messageLen := len(payloadText)
		messageHash := pii.Hash(payloadText)

		message := fcmMessage{
			Token: req.Token,
		}
		if req.Title != "" || req.Body != "" {
			message.Notification = &fcmNotification{
				Title: req.Title,
				Body:  req.Body,
			}
		}
		if len(req.Data) > 0 {
			message.Data = req.Data
		}
		requestBody := fcmRequestBody{
			Message: message,
		}
		body, err := json.Marshal(requestBody)
		if err != nil {
			log.Printf("push provider error referenceId=%q provider=%q error=%v", req.ReferenceID, PushFCMProviderName, err)
			return gateway.ProviderResult{}, err
		}

		httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, providerURL, bytes.NewReader(body))
		if err != nil {
			log.Printf("push provider error referenceId=%q provider=%q error=%v", req.ReferenceID, PushFCMProviderName, err)
			return gateway.ProviderResult{}, err
		}
		httpReq.Header.Set("Content-Type", "application/json")
		httpReq.Header.Set("Authorization", "Bearer "+token)

		log.Printf(
			"push provider request referenceId=%q provider=%q url=%q tokenMasked=%q messageLen=%d messageHash=%q",
			req.ReferenceID,
			PushFCMProviderName,
			providerURL,
			tokenMasked,
			messageLen,
			messageHash,
		)
		resp, err := client.Do(httpReq)
		if err != nil {
			log.Printf("push provider error referenceId=%q provider=%q error=%v", req.ReferenceID, PushFCMProviderName, err)
			return gateway.ProviderResult{}, err
		}
		defer resp.Body.Close()

		log.Printf("push provider response referenceId=%q provider=%q status=%d", req.ReferenceID, PushFCMProviderName, resp.StatusCode)
		if resp.StatusCode >= http.StatusOK && resp.StatusCode < http.StatusMultipleChoices {
			log.Printf("push provider decision referenceId=%q provider=%q mapped=accepted", req.ReferenceID, PushFCMProviderName)
			return gateway.ProviderResult{Status: "accepted"}, nil
		}
		errorBody := readLimitedBody(resp, fcmDebugMaxBytes)
		if errorBody != "" && req.Token != "" {
			errorBody = strings.ReplaceAll(errorBody, req.Token, tokenMasked)
		}
		// FCM signals stale/invalid device tokens as UNREGISTERED; surface a stable rejection reason so clients can drop the token.
		if isFCMUnregistered(errorBody) {
			if errorBody != "" && isFCMDebugEnabled() {
				log.Printf("push provider error body referenceId=%q provider=%q status=%d body=%q", req.ReferenceID, PushFCMProviderName, resp.StatusCode, errorBody)
			}
			log.Printf("push provider decision referenceId=%q provider=%q status=%d mapped=unregistered_token", req.ReferenceID, PushFCMProviderName, resp.StatusCode)
			return gateway.ProviderResult{Status: "rejected", Reason: "unregistered_token"}, nil
		}
		if isFCMDebugEnabled() {
			if errorBody != "" {
				log.Printf("push provider error body referenceId=%q provider=%q status=%d body=%q", req.ReferenceID, PushFCMProviderName, resp.StatusCode, errorBody)
			}
		}
		log.Printf("push provider decision referenceId=%q provider=%q status=%d mapped=provider_failure", req.ReferenceID, PushFCMProviderName, resp.StatusCode)
		return gateway.ProviderResult{}, errors.New("provider non-2xx response")
	}
}

func isFCMUnregistered(body string) bool {
	if strings.TrimSpace(body) == "" {
		return false
	}
	var parsed fcmErrorResponse
	if err := json.Unmarshal([]byte(body), &parsed); err != nil {
		return false
	}
	for _, detail := range parsed.Error.Details {
		if detail.ErrorCode == "UNREGISTERED" {
			return true
		}
	}
	return false
}

func isFCMDebugEnabled() bool {
	value := strings.TrimSpace(os.Getenv(fcmDebugEnv))
	return value == "1" || strings.EqualFold(value, "true") || strings.EqualFold(value, "yes")
}

func readLimitedBody(resp *http.Response, limit int64) string {
	if resp == nil || resp.Body == nil || limit <= 0 {
		return ""
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, limit))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

func pushPayloadSummary(req gateway.PushRequest) string {
	if req.Body != "" {
		return req.Body
	}
	if req.Title != "" {
		return req.Title
	}
	if len(req.Data) == 0 {
		return ""
	}
	keys := make([]string, 0, len(req.Data))
	for key := range req.Data {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	var b strings.Builder
	for i, key := range keys {
		if i > 0 {
			b.WriteByte('&')
		}
		b.WriteString(key)
		b.WriteByte('=')
		b.WriteString(req.Data[key])
	}
	return b.String()
}
