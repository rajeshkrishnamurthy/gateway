# SubmissionManager sync wait (waitSeconds)

## Purpose

Provide a synchronous response option on intent submission without changing execution semantics. The wait is a transport behavior only; SubmissionManager remains the sole owner of attempts, timing, and retries.

## Scope

- Applies only to `POST /v1/intents`.
- All instances behave the same; no leader or follower distinction is required for this spec.
- No contract changes; no gateway changes.

## Request

`POST /v1/intents?waitSeconds=N`

- `waitSeconds` is optional.
- `waitSeconds` is a non-negative integer in seconds.
- The request body is unchanged:
  - `intentId` (required)
  - `submissionTarget` (required)
  - `payload` (optional)

`waitSeconds` is **not** part of the idempotency key and must not be persisted on the intent.

## Behavior

### Default (async)

If `waitSeconds` is missing or `waitSeconds == 0`, the response is returned immediately after intent submission.

### Sync wait

If `waitSeconds > 0`, SubmissionManager waits up to `waitSeconds` for a meaningful state change, then returns the current intent state. The wait ends when **any** of the following occurs (first event wins):

1. The intent reaches a terminal state (`accepted`, `rejected`, or `exhausted`).
2. The first attempt completes (regardless of terminality). If the intent is still non-terminal, the response status is `pending`.
3. The wait timeout elapses.

The manager never executes attempts inline in the HTTP handler. Execution remains owned by the existing attempt loop.

### Observation mechanism

The HTTP handler observes state changes by **polling SQL persistence**. It must not rely on in-memory notifications because the executor may run in a different process. Polling reads the intent row (and attempt metadata if needed) until one of the wait termination conditions is met or the timeout elapses.

Recommended defaults:

- Poll interval: 250ms
- Max wait: 30s (server-side clamp)

### Response

The response schema is unchanged from `POST /v1/intents` and `GET /v1/intents/{intentId}`:

- `intentId`
- `submissionTarget`
- `createdAt`
- `status` (`pending`, `accepted`, `rejected`, `exhausted`)
- `completedAt` (only when terminal)
- `rejectedReason` (only when rejected)
- `exhaustedReason` (only when exhausted)

HTTP status code remains `200 OK` on success, even when the wait times out and the intent is still `pending`.

## Validation and errors

- `waitSeconds` must be an integer. Non-integer values return `400 invalid_request`.
- `waitSeconds` must be >= 0. Negative values return `400 invalid_request`.
- If `waitSeconds` exceeds the server maximum, it is clamped to that maximum (default: 30 seconds).
- All existing submission errors are returned immediately (no wait):
  - unknown submissionTarget → `400 invalid_request`
  - idempotency conflict → `409 idempotency_conflict`
  - malformed request → `400 invalid_request`

## Non-goals

- No change to contract timing semantics or retry policy.
- No guarantee of terminal response within `waitSeconds`.
- No callbacks or delivery tracking.

## Notes

- `waitSeconds` is a transport behavior only. It does not affect retry timing, acceptance deadlines, or attempt timeouts.
- Re-submitting an existing intentId with the same payload and submissionTarget is idempotent; `waitSeconds` only controls how long the caller waits to observe state changes.
