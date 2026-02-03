package submissionmanager

import (
	"context"
	"encoding/json"
	"errors"
	"time"
)

const (
	webhookPending   = "pending"
	webhookDelivered = "delivered"
	webhookFailed    = "failed"
)

func (m *Manager) dispatchWebhook(ctx context.Context, intent Intent, occurredAt time.Time) {
	if m.webhookSender == nil || intent.Contract.Webhook == nil {
		return
	}
	if intent.WebhookStatus != webhookPending {
		return
	}
	fence, ok := m.currentFence()
	if !ok {
		return
	}
	delivery, err := buildWebhookDelivery(intent, occurredAt)
	if err != nil {
		applied, err := m.store.recordWebhookAttempt(ctx, fence, intent.IntentID, webhookFailed, occurredAt, err.Error())
		if err != nil || !applied {
			if ctx == nil || ctx.Err() == nil {
				m.notifyLeaseLoss()
			}
		}
		return
	}
	if err := m.webhookSender(ctx, delivery); err != nil {
		applied, err := m.store.recordWebhookAttempt(ctx, fence, intent.IntentID, webhookFailed, occurredAt, err.Error())
		if err != nil || !applied {
			if ctx == nil || ctx.Err() == nil {
				m.notifyLeaseLoss()
			}
		}
		return
	}
	applied, err := m.store.recordWebhookAttempt(ctx, fence, intent.IntentID, webhookDelivered, occurredAt, "")
	if err != nil || !applied {
		if ctx == nil || ctx.Err() == nil {
			m.notifyLeaseLoss()
		}
	}
}

func buildWebhookDelivery(intent Intent, occurredAt time.Time) (WebhookDelivery, error) {
	webhook := intent.Contract.Webhook
	if webhook == nil {
		return WebhookDelivery{}, errors.New("webhook is not configured")
	}
	intentPayload := struct {
		IntentID         string `json:"intentId"`
		SubmissionTarget string `json:"submissionTarget"`
		CreatedAt        string `json:"createdAt"`
		CompletedAt      string `json:"completedAt"`
		Status           string `json:"status"`
		RejectedReason   string `json:"rejectedReason,omitempty"`
		ExhaustedReason  string `json:"exhaustedReason,omitempty"`
	}{
		IntentID:         intent.IntentID,
		SubmissionTarget: intent.SubmissionTarget,
		CreatedAt:        intent.CreatedAt.UTC().Format(time.RFC3339Nano),
		CompletedAt:      occurredAt.UTC().Format(time.RFC3339Nano),
		Status:           string(intent.Status),
	}
	switch intent.Status {
	case IntentRejected:
		intentPayload.RejectedReason = intent.FinalOutcome.Reason
	case IntentExhausted:
		intentPayload.ExhaustedReason = intent.ExhaustedReason
	}
	payload := struct {
		EventID    string      `json:"eventId"`
		EventType  string      `json:"eventType"`
		OccurredAt string      `json:"occurredAt"`
		Intent     interface{} `json:"intent"`
	}{
		EventID:    intent.IntentID,
		EventType:  "intent.terminal",
		OccurredAt: occurredAt.UTC().Format(time.RFC3339Nano),
		Intent:     intentPayload,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return WebhookDelivery{}, err
	}
	headers := make(map[string]string, len(webhook.Headers)+3)
	for key, value := range webhook.Headers {
		headers[key] = value
	}
	headers["Content-Type"] = "application/json"
	headers["X-Setu-Event-Type"] = "intent.terminal"
	headers["X-Setu-Event-Id"] = intent.IntentID

	headersEnv := map[string]string{}
	for key, value := range webhook.HeadersEnv {
		headersEnv[key] = value
	}

	return WebhookDelivery{
		URL:        webhook.URL,
		Headers:    headers,
		HeadersEnv: headersEnv,
		SecretEnv:  webhook.SecretEnv,
		Body:       body,
	}, nil
}
