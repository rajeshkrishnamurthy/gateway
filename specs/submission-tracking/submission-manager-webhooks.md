# SubmissionManager Webhooks (Terminal Status)
COMPLETED

## Purpose

Provide an automatic callback when an intent reaches a terminal state (accepted, rejected, or exhausted). This reduces the need for clients to poll, but does not replace GET as the source of truth.

## Scope and Non-goals

In scope:

- Terminal-only callbacks.
- SQL-backed webhook snapshots and delivery status on intents.

Out of scope:

- Delivery tracking beyond terminal intent state.
- Per-attempt callbacks.
- Changes to gateway contracts or outcomes.
- Multi-instance claiming or leader lease behavior.

## Configuration (Contract-level only)

Webhooks are configured on the submissionTarget contract and are not supplied by the client request. This avoids per-request overrides and prevents arbitrary outbound calls.

New optional contract field:

```json
{
  "submissionTarget": "sms.realtime",
  "gatewayType": "sms",
  "gatewayUrl": "http://localhost:8080",
  "policy": "deadline",
  "maxAcceptanceSeconds": 30,
  "terminalOutcomes": ["invalid_request", "invalid_recipient"],
  "webhook": {
    "url": "https://client.example.com/setu/callback",
    "headers": {
      "X-Setu-Env": "staging"
    },
    "headersEnv": {
      "Authorization": "SETU_WEBHOOK_AUTH"
    },
    "secretEnv": "SETU_WEBHOOK_SECRET"
  }
}
```

Rules:

- `webhook.url` is required when `webhook` is present.
- Only `http`/`https` URLs are allowed.
- `headers`, `headersEnv`, and `secretEnv` are optional.
- The client request must not supply or override webhook fields.
- The resolved webhook config is snapshotted on the intent and is immutable after submission.
- Secrets are not stored in the contract file. `headersEnv` and `secretEnv` reference environment variables that must be present at startup.
- Env values are resolved on delivery; if a referenced env var is missing, the webhook attempt fails with a configuration error.
- The registry-level flag `allowUnsignedWebhooks` must be true to allow a webhook without `secretEnv`. It is intended for non-production use only.
- In enterprise deployments, webhooks must be signed. Unsigned webhooks are allowed only when explicitly enabled for non-production environments.

If a contract omits `webhook`, no callback is sent.

## Delivery Semantics

Webhooks are best-effort notifications. They are not guaranteed and must not be treated as authoritative.

- A webhook is scheduled when an intent transitions to a terminal state.
- Webhook delivery must not change intent status.
- The intent is persisted first; webhook delivery happens after persistence.
- Delivery is best-effort and may be missed (for example, a crash between persistence and enqueue).
- Clients must treat GET `/v1/intents/{intentId}` as the source of truth.

## Webhook Request

Method and headers:

- `POST <webhook.url>`
- `Content-Type: application/json`
- `X-Setu-Event-Type: intent.terminal`
- `X-Setu-Event-Id: <intentId>`
- `X-Setu-Signature: <hmac>` when `secret` is configured

Signature:

- `X-Setu-Signature` is HMAC-SHA256 of the raw request body using the resolved `secretEnv` value.

Payload:

```json
{
  "eventId": "intent-1",
  "eventType": "intent.terminal",
  "occurredAt": "2026-02-02T17:17:10.775Z",
  "intent": {
    "intentId": "intent-1",
    "submissionTarget": "sms.realtime",
    "createdAt": "2026-02-02T17:16:00.000Z",
    "completedAt": "2026-02-02T17:17:10.775Z",
    "status": "rejected",
    "rejectedReason": "invalid_recipient",
    "exhaustedReason": ""
  }
}
```

Notes:

- `status` is always terminal (`accepted`, `rejected`, `exhausted`).
- `rejectedReason` is present only when rejected.
- `exhaustedReason` is present only when exhausted.
- `eventId` and `X-Setu-Event-Id` are the intentId because there is exactly one terminal webhook per intent. If additional event types are introduced later, eventId will become a unique per-event identifier and must be treated as opaque.

## Success and Failure

- Any 2xx response is treated as delivered.
- Network errors or non-2xx responses are recorded as failed.
- No retries are attempted in this phase.

## Persistence

Webhook state is persisted on the intent row. Suggested fields:

- intent_id (PK, FK)
- url, headers_json, headers_env_json, secret_env
- status (pending, delivered, failed)
- last_error
- attempted_at
- delivered_at

This persistence model assumes a single terminal webhook per intent and a single delivery attempt. If retries or multiple webhook events per intent are introduced, move to a dedicated webhook table.

## Interaction With Sync Wait

The `waitSeconds` HTTP parameter only affects the client response. Webhook delivery is independent and may occur before or after the client response returns.
