package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"gateway/submission"
	"gateway/submissionmanager"
)

type gatewayResponse struct {
	Status string `json:"status"`
	Reason string `json:"reason"`
}

func newGatewayExecutor(client *http.Client) submissionmanager.AttemptExecutor {
	if client == nil {
		client = http.DefaultClient
	}
	return func(ctx context.Context, input submissionmanager.AttemptInput) (submissionmanager.GatewayOutcome, error) {
		endpoint, err := gatewayEndpoint(input.GatewayType, input.GatewayURL)
		if err != nil {
			return submissionmanager.GatewayOutcome{}, err
		}
		payload := input.Payload
		if payload == nil {
			payload = []byte{}
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
		if err != nil {
			return submissionmanager.GatewayOutcome{}, err
		}
		req.Header.Set("Content-Type", "application/json")

		resp, err := client.Do(req)
		if err != nil {
			return submissionmanager.GatewayOutcome{}, err
		}
		defer resp.Body.Close()

		if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
			_, _ = io.Copy(io.Discard, resp.Body)
			return submissionmanager.GatewayOutcome{}, fmt.Errorf("gateway returned status %d", resp.StatusCode)
		}

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return submissionmanager.GatewayOutcome{}, err
		}
		var gatewayResp gatewayResponse
		if err := json.Unmarshal(body, &gatewayResp); err != nil {
			return submissionmanager.GatewayOutcome{}, fmt.Errorf("decode gateway response: %w", err)
		}

		return submissionmanager.GatewayOutcome{
			Status: strings.TrimSpace(gatewayResp.Status),
			Reason: strings.TrimSpace(gatewayResp.Reason),
		}, nil
	}
}

func gatewayEndpoint(gatewayType submission.GatewayType, baseURL string) (string, error) {
	trimmed := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if trimmed == "" {
		return "", fmt.Errorf("gateway url is required")
	}
	switch gatewayType {
	case submission.GatewaySMS:
		return trimmed + "/sms/send", nil
	case submission.GatewayPush:
		return trimmed + "/push/send", nil
	default:
		return "", fmt.Errorf("unknown gateway type %q", gatewayType)
	}
}
