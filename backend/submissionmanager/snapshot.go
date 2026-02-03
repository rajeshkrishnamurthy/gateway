package submissionmanager

import (
	"encoding/json"

	"gateway/submission"
)

func normalizePayload(payload json.RawMessage) json.RawMessage {
	if payload == nil {
		return []byte{}
	}
	return clonePayload(payload)
}

func clonePayload(payload json.RawMessage) json.RawMessage {
	copyPayload := make([]byte, len(payload))
	copy(copyPayload, payload)
	return copyPayload
}

func cloneContract(contract submission.TargetContract) submission.TargetContract {
	clone := contract
	if len(contract.TerminalOutcomes) > 0 {
		clone.TerminalOutcomes = append([]string(nil), contract.TerminalOutcomes...)
	}
	if contract.Webhook != nil {
		clone.Webhook = cloneWebhook(contract.Webhook)
	}
	return clone
}

func cloneWebhook(webhook *submission.WebhookConfig) *submission.WebhookConfig {
	if webhook == nil {
		return nil
	}
	clone := &submission.WebhookConfig{
		URL:       webhook.URL,
		SecretEnv: webhook.SecretEnv,
	}
	if len(webhook.Headers) > 0 {
		clone.Headers = make(map[string]string, len(webhook.Headers))
		for key, value := range webhook.Headers {
			clone.Headers[key] = value
		}
	}
	if len(webhook.HeadersEnv) > 0 {
		clone.HeadersEnv = make(map[string]string, len(webhook.HeadersEnv))
		for key, value := range webhook.HeadersEnv {
			clone.HeadersEnv[key] = value
		}
	}
	return clone
}
