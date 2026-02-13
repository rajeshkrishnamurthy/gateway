package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	setulog "gateway/logging"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

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
		start := time.Now()
		logger := slog.Default().With(
			"component", "submission-manager-executor",
			"commProfile", setulog.CommProfileSetuToSetu,
			"callerService", "submission-manager",
			"calleeService", calleeServiceForGateway(input.GatewayType),
			"operation", "gateway_submit",
			"intentId", input.IntentID,
		)
		endpoint, err := gatewayEndpoint(input.GatewayType, input.GatewayURL)
		if err != nil {
			logger.Error(
				"gateway endpoint resolution failed",
				"event", "submission.gateway_call.failed",
				"outcome", "endpoint_error",
				"durationMs", time.Since(start).Seconds()*1000,
				"error", err,
			)
			return submissionmanager.GatewayOutcome{}, err
		}
		payload := input.Payload
		if payload == nil {
			payload = []byte{}
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
		if err != nil {
			logger.Error(
				"gateway request build failed",
				"event", "submission.gateway_call.failed",
				"outcome", "request_build_error",
				"durationMs", time.Since(start).Seconds()*1000,
				"error", err,
			)
			return submissionmanager.GatewayOutcome{}, err
		}
		req.Header.Set("Content-Type", "application/json")

		resp, err := client.Do(req)
		if err != nil {
			logger.Error(
				"gateway call failed",
				"event", "submission.gateway_call.failed",
				"outcome", "transport_error",
				"durationMs", time.Since(start).Seconds()*1000,
				"error", err,
			)
			return submissionmanager.GatewayOutcome{}, err
		}
		defer resp.Body.Close()

		if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
			_, _ = io.Copy(io.Discard, resp.Body)
			logger.Error(
				"gateway call non-2xx",
				"event", "submission.gateway_call.failed",
				"outcome", "http_error",
				"statusCode", resp.StatusCode,
				"durationMs", time.Since(start).Seconds()*1000,
			)
			return submissionmanager.GatewayOutcome{}, fmt.Errorf("gateway returned status %d", resp.StatusCode)
		}

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			logger.Error(
				"gateway response read failed",
				"event", "submission.gateway_call.failed",
				"outcome", "read_error",
				"statusCode", resp.StatusCode,
				"durationMs", time.Since(start).Seconds()*1000,
				"error", err,
			)
			return submissionmanager.GatewayOutcome{}, err
		}
		var gatewayResp gatewayResponse
		if err := json.Unmarshal(body, &gatewayResp); err != nil {
			logger.Error(
				"gateway response decode failed",
				"event", "submission.gateway_call.failed",
				"outcome", "decode_error",
				"statusCode", resp.StatusCode,
				"durationMs", time.Since(start).Seconds()*1000,
				"error", err,
			)
			return submissionmanager.GatewayOutcome{}, fmt.Errorf("decode gateway response: %w", err)
		}

		outcomeStatus := strings.TrimSpace(gatewayResp.Status)
		if outcomeStatus == "" {
			outcomeStatus = "unknown"
		}
		logger.Info(
			"gateway call completed",
			"event", "submission.gateway_call.completed",
			"outcome", outcomeStatus,
			"statusCode", resp.StatusCode,
			"gatewayReason", strings.TrimSpace(gatewayResp.Reason),
			"durationMs", time.Since(start).Seconds()*1000,
		)

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

func calleeServiceForGateway(gatewayType submission.GatewayType) string {
	switch gatewayType {
	case submission.GatewaySMS:
		return "sms-gateway"
	case submission.GatewayPush:
		return "push-gateway"
	default:
		return "unknown-gateway"
	}
}
