# Gateway contracts

## Purpose

This document specifies the submission-only behavior of the SMS and push gateways. It describes request/response schemas, validation, idempotency scope, and HTTP endpoints. These gateways do not own retries or delivery tracking.

## Submission-only semantics

- An accepted response means the gateway submitted the request to the provider.
- The gateway does not track delivery or retries after acceptance.
- Rejections represent validation failures or provider submission failures only.

## HTTP endpoints

Both gateways expose the following endpoints:

- `POST /sms/send` or `POST /push/send` for submission.
- `GET /healthz` and `GET /readyz` for basic health checks.
- `GET /metrics` for Prometheus metrics (404 if metrics are disabled).
- `GET /ui` and `GET /ui/*` for the HTML console when UI templates are present.
- `GET /ui/static/*` for static assets.

The request body size for send endpoints is capped at 16 KiB.

### HTMX behavior

If the request includes `HX-Request: true`, send endpoints return HTML fragments instead of JSON using `send_result.tmpl`. When a JSON response would be `4xx`, the fragment is still returned with `200` to keep HTMX swaps stable. Non-HTMX requests receive standard JSON responses.

## SMS gateway contract

### Request

```json
{
  "referenceId": "string",
  "to": "string",
  "message": "string",
  "tenantId": "string (optional)"
}
```

### Response

```json
{
  "referenceId": "string",
  "status": "accepted|rejected",
  "gatewayMessageId": "string (present when accepted)",
  "reason": "invalid_request|duplicate_reference|invalid_recipient|invalid_message|provider_failure (present when rejected)"
}
```

### Validation

The SMS gateway rejects requests as `invalid_request` when:

- `referenceId` is empty
- `to` is empty
- `message` is empty
- JSON decoding fails or contains trailing data

### Idempotency scope

Idempotency is enforced only for concurrent in-flight requests within the same process. A duplicate `referenceId` while in-flight is rejected with reason `duplicate_reference`. There is no durable idempotency across time or restarts.

### Provider failure handling

Any provider error or panic is normalized to `rejected` with reason `provider_failure`.

### HTTP status codes

- Gateways must never return 2xx unless they can produce a complete, valid normalized outcome.
- `200` for any normalized outcome (accepted or rejected).
- non‑2xx only when a normalized outcome cannot be produced.

## Push gateway contract

### Request

```json
{
  "referenceId": "string",
  "token": "string",
  "title": "string (optional)",
  "body": "string (optional)",
  "data": {"key": "value"} (optional),
  "tenantId": "string (optional)"
}
```

At least one of `title`, `body`, or `data` must be present.

### Response

```json
{
  "referenceId": "string",
  "status": "accepted|rejected",
  "gatewayMessageId": "string (present when accepted)",
  "reason": "invalid_request|duplicate_reference|provider_failure|unregistered_token (present when rejected)"
}
```

### Validation

The push gateway rejects requests as `invalid_request` when:

- `referenceId` is empty
- `token` is empty
- `title`, `body`, and `data` are all empty
- JSON decoding fails or contains trailing data

### Idempotency scope

Idempotency is enforced only for concurrent in-flight requests within the same process. A duplicate `referenceId` while in-flight is rejected with reason `duplicate_reference`. There is no durable idempotency across time or restarts.

### Provider failure handling

Any provider error or panic is normalized to `rejected` with reason `provider_failure`.

### HTTP status codes

- Gateways must never return 2xx unless they can produce a complete, valid normalized outcome.
- `200` for any normalized outcome (accepted or rejected).
- non‑2xx only when a normalized outcome cannot be produced.

## Gateway message IDs

When a response is accepted, the gateway generates a `gatewayMessageId` locally. This ID represents confirmed submission, not delivery, and should not be assumed to exist when the provider outcome is unknown.

## Logging

Both gateways emit a decision log entry per submission attempt with:

- `referenceId`
- `status`
- `reason`
- `source` (validation, provider_result, or provider_failure)
- `gatewayMessageId` (when present)

Provider adapter logging requirements are specified in `model-provider-adapter.md` and `backend/adapter/AGENTS.md`.

## UI console

The gateway UI console includes:

- an overview page
- a send test form
- a metrics view derived from Prometheus text output
