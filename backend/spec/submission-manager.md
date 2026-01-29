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

SubmissionManager owns time and attempts. It executes the first attempt immediately, schedules retries only if the contract allows, and completes intents as ACCEPTED, REJECTED, or EXHAUSTED. It does not reinterpret gateway semantics and does not reason about delivery after acceptance.

#### Execution engine (Phase 2)

Phase 2 adds an in-memory SubmissionManager execution engine (no HTTP surface, no persistence). It resolves submissionTarget into a contract snapshot at submission time and stores that snapshot on the intent. Routing is explicit: the resolved gatewayType and gatewayUrl are passed into the executor, and execution does not re-resolve them.

Intent status is an orchestration outcome derived from gateway outcomes plus contract policy:

- ACCEPTED: gateway outcome was accepted and, for deadline policy, the acceptance occurred before the acceptance deadline.
- REJECTED: gateway outcome was rejected and the rejection reason is a terminalOutcome for the contract.
- EXHAUSTED: policy termination was reached without acceptance or terminal rejection (deadline exceeded, max attempts reached, or one-shot completed).

Additional semantics:

- FinalOutcome is meaningful only for ACCEPTED and REJECTED intents.
- ExhaustedReason explains policy exhaustion, not gateway failure.
- Only rejection reasons explicitly listed in terminalOutcomes are terminal. All other rejection reasons are treated as non-terminal and retryable under policy, while still being recorded on attempts.
- Terminal intents are append-only: once ACCEPTED, REJECTED, or EXHAUSTED, no further attempts run and the outcome does not change.
- A repeated intentId with the same submissionTarget and payload is idempotent and returns the existing intent.
- A repeated intentId with a different submissionTarget or payload is an idempotency conflict.
- Invalid gateway outcomes (missing status, missing rejection reason, or unknown status) are recorded as attempt errors and treated as non-terminal under policy.
- Phase 2 idempotency and intent state are in-memory only; process restarts clear intent history.

Retry timing:

- Phase 2 uses a fixed 5 second retry delay as an internal execution policy.
- Retry timing is not a contract term and must not be surfaced as part of submissionTarget semantics.

## SubmissionTarget Registry

The registry is a JSON config that defines contracts for each submissionTarget. It binds each submissionTarget to a gatewayType and a gatewayUrl (HAProxy frontend) and captures contract constraints.

The registry is the single source of truth for submissionTarget to gatewayType binding.

## Contract Fields

Each registry entry defines:

- submissionTarget: stable, unique target identifier
- gatewayType: sms or push (code-known)
- gatewayUrl: base URL for the HAProxy frontend for this gateway type
- mode: realtime or batch
- policy: one of `deadline`, `max_attempts`, or `one_shot`
- maxAcceptanceSeconds: required when policy is `deadline`
- maxAttempts: required when policy is `max_attempts`
- terminalOutcomes: gateway-reported outcomes that this contract treats as terminal

Notes:

- terminalOutcomes are contract semantics. They do not change gateway behavior.
- accepted is always terminal and is not listed in terminalOutcomes.
- maxAcceptanceSeconds is a cumulative bound across all attempts, not a per-attempt timeout.
- for deadline policy, a retry is scheduled only if the next due time is strictly before the acceptance deadline; otherwise the intent is exhausted.
- fields not required by the selected policy must be omitted.
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
      "mode": "realtime",
      "policy": "deadline",
      "maxAcceptanceSeconds": 30,
      "terminalOutcomes": ["invalid_request", "invalid_recipient", "invalid_message"]
    },
    {
      "submissionTarget": "push.realtime",
      "gatewayType": "push",
      "gatewayUrl": "http://localhost:8081",
      "mode": "realtime",
      "policy": "deadline",
      "maxAcceptanceSeconds": 30,
      "terminalOutcomes": ["invalid_request", "unregistered_token"]
    }
  ]
}
```
