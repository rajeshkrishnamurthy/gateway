# Gateway HTTP status normalization

This execplan is a living document. The sections Progress, Surprises & Discoveries, Decision Log, and Outcomes & Retrospective must be kept up to date as work proceeds.

This plan follows `backend/PLANS.md` from the repository root and must be maintained in accordance with it.

## Purpose / Big Picture

After this change, the gateway will return HTTP 2xx for any normalized outcome (accepted or rejected) and reserve non‑2xx responses only for cases where it cannot produce a normalized outcome. This keeps the contract simple for internal clients while preserving SubmissionManager semantics. The SubmissionManager HTTP executor will parse outcomes only on 2xx and treat non‑2xx as attempt errors. Success is observable by sending an invalid request to the gateway and receiving HTTP 200 with `status=rejected`, while a true internal error yields a non‑2xx response and the executor records an attempt error.

## Progress

- [x] (2026-01-29 18:11Z) Review existing gateway status semantics in `specs/gateway-contracts.md` and `backend/README.md`.
- [x] (2026-01-29 18:11Z) Update gateway handlers to return 2xx for all normalized outcomes (accepted or rejected) and reserve non‑2xx for non‑normalizable failures.
- [x] (2026-01-29 18:11Z) Update gateway tests to expect 2xx for rejected outcomes (including invalid_request and duplicate_reference).
- [x] (2026-01-29 18:11Z) Update the SubmissionManager executor to parse JSON outcomes only on 2xx and treat non‑2xx as attempt errors.
- [x] (2026-01-29 18:11Z) Add executor tests for non‑2xx handling and JSON parsing rules.
- [x] (2026-01-29 18:11Z) Update specs and README to document the new HTTP status semantics.

## Surprises & Discoveries

None yet.

## Decision Log

- Decision: Gateways return HTTP 2xx for any normalized outcome (accepted or rejected), including invalid_request and duplicate_reference; non‑2xx is reserved for cases where a normalized outcome cannot be produced or encoded.
  Rationale: Simplifies the internal contract and aligns with the idea that rejected is a valid outcome, not a transport failure.
  Date/Author: 2026-01-29 / Codex

- Decision: No structured error body is required for non‑2xx responses at this stage; a plain error body is acceptable.
  Rationale: Keeps the change minimal while preserving the internal contract.
  Date/Author: 2026-01-29 / Codex

- Decision: The SubmissionManager HTTP executor parses JSON outcomes only on 2xx; non‑2xx responses are treated as attempt errors.
  Rationale: Aligns the executor with the new gateway contract and keeps retry semantics stable.
  Date/Author: 2026-01-29 / Codex

## Outcomes & Retrospective

Gateway HTTP status semantics now return 2xx for all normalized outcomes and reserve non‑2xx for non‑normalizable failures. The SubmissionManager executor aligns with this by parsing JSON only on 2xx and treating non‑2xx as attempt errors. Specs and README are updated, and both gateway handler tests and executor tests cover the new behavior.

## Context and Orientation

Gateway HTTP behavior is defined in `specs/gateway-contracts.md` and summarized in the “HTTP status semantics” section of `backend/README.md`. SMS and push gateway handlers live in `backend/cmd/sms-gateway/main.go` and `backend/cmd/push-gateway/main.go`. The SubmissionManager HTTP adapter (Phase 3b) lives in `backend/cmd/submission-manager/` and uses `executor.go` to make gateway HTTP calls and parse normalized outcomes.

Normalized outcomes are the gateway JSON responses with `status` (accepted|rejected) and `reason` (when rejected). SubmissionManager relies on those normalized outcomes and should not depend on HTTP status codes beyond error detection.

## Plan of Work

Update the gateway contract to treat all normalized outcomes as HTTP 2xx responses. That means the SMS and push HTTP handlers should return 200 for accepted and rejected outcomes, including invalid_request and duplicate_reference cases. Only internal failures where the handler cannot produce or encode a normalized response should produce non‑2xx codes (e.g., 500). Update the gateway tests to reflect this new rule.

Update the SubmissionManager HTTP executor to only parse JSON when the gateway returns 2xx. If the response is non‑2xx, treat it as an attempt error and let policy decide retries. If a 2xx response cannot be parsed as JSON or is missing required fields, treat that as an attempt error (as today).

Update the documentation to reflect the new HTTP status contract and keep the spec canonical. Ensure tests demonstrate that invalid requests now return 200 with `status=rejected` and that non‑2xx is reserved for non‑normalizable failures.

## Concrete Steps

1) Read and update `specs/gateway-contracts.md` to state that normalized outcomes always return 2xx and non‑2xx is reserved for non‑normalizable failures.

2) Update `backend/README.md` “HTTP status semantics” to align with the new contract (rejected is still a valid response, now always 2xx).

3) Modify gateway handlers:
   - `backend/cmd/sms-gateway/main.go` (handler `handleSMSSend` and response writing) to return 200 for rejected outcomes (including invalid_request, duplicate_reference, invalid_recipient, invalid_message, provider_failure).
   - `backend/cmd/push-gateway/main.go` (handler `handlePushSend`) with the same 200-for-rejected behavior.
   - Keep non‑2xx only for cases where a normalized outcome cannot be produced or encoded.

4) Update gateway tests to expect 200 for rejected outcomes and adjust any assertions that relied on 400 for invalid_request or duplicate_reference.

5) Update `backend/cmd/submission-manager/executor.go` to parse JSON outcomes only for 2xx responses. For non‑2xx, return an attempt error that includes the HTTP status code. Do not change SubmissionManager policy logic.

6) Add executor tests in `backend/cmd/submission-manager/` to verify:
   - 2xx responses with valid JSON produce gateway outcomes.
   - non‑2xx responses produce attempt errors.
   - malformed JSON on 2xx yields attempt error.

7) Run tests from `backend/`:

   go test ./...

## Validation and Acceptance

- Sending an invalid request to SMS or push gateway returns HTTP 200 with JSON `status=rejected` and appropriate `reason`.
- Duplicate_reference responses are HTTP 200 with `status=rejected`.
- Internal failures (non‑normalizable responses) return non‑2xx and are treated as attempt errors by the SubmissionManager executor.
- All tests pass under `go test ./...`.

## Idempotence and Recovery

This change is additive and safe to repeat. It modifies handler response codes and executor parsing rules but does not change database schema or state. If tests fail, revert the handler status code changes and executor parsing changes together to restore previous semantics.

## Artifacts and Notes

Example HTTP response after change for invalid request:

  200 OK
  {
    "status": "rejected",
    "reason": "invalid_request"
  }

Example non‑normalizable failure (internal error):

  500 Internal Server Error
  <plain text or empty body>

## Interfaces and Dependencies

- Gateway handlers: `backend/cmd/sms-gateway/main.go`, `backend/cmd/push-gateway/main.go`
- Specs: `specs/gateway-contracts.md`, `backend/README.md`
- Executor: `backend/cmd/submission-manager/executor.go`
- Tests: gateway handler tests and new executor tests under `backend/cmd/submission-manager/`

---

Change log:
- execplan created to normalize gateway HTTP statuses to 2xx for all normalized outcomes and align executor parsing rules. (2026-01-29 / Codex)
- Progress updated with implementation status, documentation changes, and completed tests for gateway handlers and executor. (2026-01-29 / Codex)
