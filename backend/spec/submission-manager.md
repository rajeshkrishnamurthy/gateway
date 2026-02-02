# Submission Manager and SubmissionTarget Contracts

## Purpose

This document defines SubmissionIntent, SubmissionTarget contracts, and the registry that binds targets to gateway types and routing URLs. It separates gateway semantics from contract constraints and clarifies that SubmissionManager owns time and attempts without reinterpreting gateway behavior.

## Terms and Roles

### SubmissionIntent

A SubmissionIntent is the client-facing unit of work. It contains:

- intentId (stable idempotency key)
- submissionTarget (selects the contract)
- payload (opaque data forwarded to the gateway type)

The client is oblivious to attempts. Attempt scheduling and retries are owned by SubmissionManager.

### gatewayType

gatewayType is code-known and defines protocol and response semantics for a gateway family (for example: sms, push). SubmissionManager uses gatewayType to parse and interpret gateway responses. gatewayType is not supplied by the client; it is bound in the registry.

### submissionTarget

submissionTarget is data-driven and selects a contract. It is the contract identifier and does not encode gatewayType by convention. The registry provides the explicit submissionTarget to gatewayType binding.

### SubmissionManager

SubmissionManager owns time and attempts. It executes the first attempt immediately, schedules retries only if the contract allows, and completes intents as ACCEPTED, REJECTED, or EXHAUSTED. While execution is in progress, intents remain in PENDING. It does not reinterpret gateway semantics and does not reason about delivery after acceptance.

#### Execution engine

SubmissionManager executes intents and persists state in SQL Server for intents, attempts, and scheduling metadata. It resolves submissionTarget into a contract snapshot at submission time and stores that snapshot on the intent. Routing is explicit: the resolved gatewayType and gatewayUrl are passed into the executor, and execution does not re-resolve them.

Intent status is an orchestration outcome derived from gateway outcomes plus contract policy:

- ACCEPTED: gateway outcome was accepted and, for deadline policy, the acceptance occurred before the acceptance deadline.
- REJECTED: gateway outcome was rejected and the rejection reason is a terminalOutcome for the contract.
- EXHAUSTED: policy termination was reached without acceptance or terminal rejection (deadline exceeded, max attempts reached, or one-shot completed).
- PENDING: intent is still executing or waiting for the next attempt.

Additional semantics:

- RejectedReason is meaningful only for REJECTED intents.
- ExhaustedReason explains policy exhaustion, not gateway failure.
- CompletedAt records when an intent reached a terminal state (accepted, rejected, exhausted).
- Only rejection reasons explicitly listed in terminalOutcomes are terminal. All other rejection reasons are treated as non-terminal and retryable under policy, while still being recorded on attempts.
- Terminal intents are append-only: once ACCEPTED, REJECTED, or EXHAUSTED, no further attempts run and the outcome does not change.
- A repeated intentId with the same submissionTarget and payload is idempotent and returns the existing intent.
- A repeated intentId with a different submissionTarget or payload is an idempotency conflict.
- Payload is persisted as raw bytes along with a hash to enforce idempotency across restarts.
- Invalid gateway outcomes (missing status, missing rejection reason, or unknown status) are recorded as attempt errors and treated as non-terminal under policy.
- Intent state, attempts, and nextAttemptAt are persisted in SQL Server; restarts rebuild the in-memory queue from persisted schedule data.
- The resolved contract snapshot (submissionTarget, gatewayType, gatewayUrl, policy, terminalOutcomes) is persisted per intent; contract masters remain file-based.
- A single SubmissionManager process is assumed; no worker claiming, leasing, or multi-instance coordination is introduced.
- attempt_count on the intent row is the authoritative attempt number source; the attempts table is an audit log and must not be used to derive attempt sequencing.

Retry timing:

- A fixed 5 second retry delay is used as an internal execution policy.
- Retry timing is not a contract term and must not be surfaced as part of submissionTarget semantics.

#### Persistence

Intent state, attempts, and scheduling metadata are stored in SQL Server. The schema lives in `backend/conf/sql/submissionmanager/001_create_schema.sql` and includes:

- `submission_intents` with the contract snapshot, payload, payload_hash, status, attempt_count, and next_attempt_at.
- `submission_attempts` as an append-only audit log for each attempt.

SubmissionManager rebuilds its in-memory schedule on startup from `next_attempt_at`, and idempotency checks are enforced against persisted payloads.

#### HTTP API

SubmissionManager is exposed over HTTP as a thin adapter with no semantic changes. The HTTP layer must not expose the contract snapshot and must delegate directly to the existing manager methods.

Endpoints:

- GET `/healthz`, `/readyz` return `200 OK` when the process is running.
- GET `/metrics` returns Prometheus metrics for SubmissionManager in text format. Metrics are prefixed with `submission_` and do not duplicate gateway metrics.
- POST `/v1/intents` creates or queries an intent (idempotent). Request JSON:
  - intentId (string, required)
  - submissionTarget (string, required)
  - payload (opaque JSON, optional)
  Response JSON includes intentId, submissionTarget, createdAt, status, completedAt (when terminal), rejectedReason (when rejected), and exhaustedReason (when exhausted). Status values are: pending, accepted, rejected, exhausted.
- GET `/v1/intents/{intentId}` returns the current intent state or 404 if unknown.
- GET `/v1/intents/{intentId}/history` returns the current intent state plus the ordered attempt history. The response includes an `intent` object (same shape as `/v1/intents/{intentId}`) and an `attempts` array (attemptNumber, startedAt, finishedAt, outcomeStatus, outcomeReason, error).

Error mapping:

- 400 invalid_request for malformed JSON, missing intentId/submissionTarget, or unknown submissionTarget.
- 404 not_found when an intentId does not exist.
- 409 idempotency_conflict when the same intentId is reused with a different payload or submissionTarget.
- 500 internal_error for unexpected failures.

#### Intent history UI

SubmissionManager exposes a history fragment endpoint at `/ui/history`. It accepts POST form data with `intentId` and returns an HTML fragment with the intent summary and attempts table. This view is authoritative because it is sourced from SQL persistence.

## SubmissionTarget Registry

The registry is a JSON config that defines contracts for each submissionTarget. It binds each submissionTarget to a gatewayType and a gatewayUrl (HAProxy frontend) and captures contract constraints.

The registry is the single source of truth for submissionTarget to gatewayType binding.

## Contract Fields

Each registry entry defines:

- submissionTarget: stable, unique target identifier
- gatewayType: sms or push (code-known)
- gatewayUrl: base URL for the HAProxy frontend for this gateway type (http/https with host)
- policy: one of `deadline`, `max_attempts`, or `one_shot`
- maxAcceptanceSeconds: required when policy is `deadline`
- maxAttempts: required when policy is `max_attempts`
- terminalOutcomes: required list of gateway-reported outcomes that this contract treats as terminal

Notes:

- terminalOutcomes are contract semantics. They do not change gateway behavior.
- accepted is always terminal and is not listed in terminalOutcomes.
- maxAcceptanceSeconds is a cumulative bound across all attempts, not a per-attempt timeout.
- for deadline policy, a retry is scheduled only if the next due time is strictly before the acceptance deadline; otherwise the intent is exhausted.
- fields not required by the selected policy must be omitted.
- terminalOutcomes must not include empty values, must be unique, and must be valid for the gatewayType.
- policy selects the retry termination rule:
  - `deadline`: retries are allowed until the acceptance deadline.
  - `max_attempts`: retries are allowed until the attempt count reaches maxAttempts.
  - `one_shot`: only a single attempt is made.

## Gateway Outcome Taxonomy

### SMS

Gateway response status is accepted or rejected. Rejection reasons include:

- invalid_request
- duplicate_reference
- invalid_recipient
- invalid_message
- provider_failure

### Push

Gateway response status is accepted or rejected. Rejection reasons include:

- invalid_request
- duplicate_reference
- provider_failure
- unregistered_token

## Registry Example

```json
{
  "targets": [
    {
      "submissionTarget": "sms.realtime",
      "gatewayType": "sms",
      "gatewayUrl": "http://localhost:8080",
      "policy": "deadline",
      "maxAcceptanceSeconds": 30,
      "terminalOutcomes": ["invalid_request", "invalid_recipient", "invalid_message"]
    },
    {
      "submissionTarget": "push.realtime",
      "gatewayType": "push",
      "gatewayUrl": "http://localhost:8081",
      "policy": "deadline",
      "maxAcceptanceSeconds": 30,
      "terminalOutcomes": ["invalid_request", "unregistered_token"]
    }
  ]
}
```
