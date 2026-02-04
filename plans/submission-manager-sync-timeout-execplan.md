# Implement waitSeconds sync wait for POST /v1/intents

This execplan is a living document. The sections `Progress`, `Surprises & Discoveries`, `Decision Log`, and `Outcomes & Retrospective` must be kept up to date as work proceeds.

This plan must be maintained in accordance with `backend/PLANS.md` from the repository root.

## Purpose / Big Picture

After this change, a client can submit an intent and optionally ask the HTTP call to wait for a short period to observe the first attempt or terminal state. The execution engine remains unchanged and owned by SubmissionManager; the HTTP handler only waits by polling SQL persistence. The behavior can be verified by sending a `POST /v1/intents?waitSeconds=...` request and observing that the response returns immediately when the first attempt completes (or when the intent reaches terminal), without changing retry or contract semantics.

## Progress

- [x] (2026-02-02 12:45Z) Drafted execplan for sync waitSeconds behavior.
- [x] (2026-02-02 13:05Z) Implemented waitSeconds parsing/validation in `backend/cmd/submission-manager/handlers.go` and wired it into the submit path.
- [x] (2026-02-02 13:05Z) Added SQL-backed wait polling in `backend/submissionmanager/manager.go`.
- [x] (2026-02-02 13:10Z) Added waitSeconds tests (invalid, negative, timeout, early return) in `backend/cmd/submission-manager/main_test.go`.
- [x] (2026-02-02 13:12Z) Updated `specs/submission-manager.md` to reference `specs/manager-sync-timeout.md`.
- [x] (2026-02-02 13:15Z) Ran `go test ./cmd/submission-manager -count=1`.

## Surprises & Discoveries

- None yet.

## Decision Log

- Decision: Implement sync waits by polling SQL persistence only, not in-memory notifications.
  Rationale: The executor may be in a different process in future leader-lease deployments, so SQL is the only shared source of truth.
  Date/Author: 2026-02-02 / Codex
- Decision: Use a single `waitSeconds` query parameter with server-side clamp (default max 30s) and a fixed poll interval (250ms).
  Rationale: Keeps the API minimal while still allowing short synchronous waits without exposing execution semantics.
  Date/Author: 2026-02-02 / Codex
- Decision: End the wait when the intent becomes terminal or when the first attempt completes (attempt_count >= 1), whichever comes first.
  Rationale: Matches the spec in `specs/manager-sync-timeout.md` and keeps sync behavior transport-only.
  Date/Author: 2026-02-02 / Codex
- Decision: Update test contract to `max_attempts` so the first attempt can complete without terminal status.
  Rationale: Enables a test that proves waitSeconds returns after the first attempt even when the intent remains pending.
  Date/Author: 2026-02-02 / Codex

## Outcomes & Retrospective

Sync waitSeconds support is implemented end-to-end: the HTTP handler validates and clamps the query param, SubmissionManager polls SQL for terminal or first-attempt completion, and tests cover validation, timeout, and early return. The behavior stays transport-only and does not alter executor semantics. Tests pass with SQL Server-backed integration coverage.

## Context and Orientation

SubmissionManager is implemented in `backend/submissionmanager/manager.go` with SQL persistence in `backend/submissionmanager/store.go`. The HTTP API is in `backend/cmd/submission-manager/handlers.go`, registered in `backend/cmd/submission-manager/routes.go`. `POST /v1/intents` currently returns immediately after `SubmitIntent` without waiting. The canonical sync-wait behavior is specified in `specs/manager-sync-timeout.md` and the core SubmissionManager semantics live in `specs/submission-manager.md`.

Key types and functions:

- `submissionmanager.Manager.SubmitIntent` inserts the intent and schedules the first attempt.
- `submissionmanager.Manager.GetIntent` loads intent state from SQL, including attempts.
- `sqlStore.loadIntentRow` loads the intent row (status, attempt_count, updated_at) without attempts; it already exists in `backend/submissionmanager/store.go`.
- HTTP handler `apiServer.handleSubmit` lives in `backend/cmd/submission-manager/handlers.go` and currently calls `SubmitIntent` then returns `toIntentResponse`.

In this repo, “attempt_count” in `dbo.submission_intents` is the authoritative indicator that the first attempt completed; it is updated at attempt record time and should be used for the wait condition.

## Plan of Work

Start by adding waitSeconds handling to the HTTP handler and keep all waiting logic inside the submissionmanager package so the HTTP layer remains an adapter. Introduce a new Manager method that polls SQL for intent state and attempt_count until a terminal state or first attempt completion is seen, or the wait timeout elapses. Use the request context so the wait stops if the client disconnects; treat context cancellation the same as timeout by returning the most recent intent state. Add constants for max wait seconds and poll interval in the HTTP handler (max wait clamp) and in the manager method (poll interval). Ensure no changes to executor logic or contract semantics.

Update `specs/submission-manager.md` to mention the `waitSeconds` query parameter on `POST /v1/intents` and explicitly point readers to `specs/manager-sync-timeout.md` for detailed sync-wait semantics. No behavior changes elsewhere.

Write tests in `backend/cmd/submission-manager/main_test.go` that cover:

1. Validation: non-integer and negative waitSeconds return `400 invalid_request`.
2. Timeout: with no executor running, a small waitSeconds returns `pending` after the timeout.
3. Early return: start `Manager.Run` in a goroutine, submit with waitSeconds, and assert the response is terminal (accepted/rejected/exhausted) once the first attempt completes.

## Concrete Steps

1) Update HTTP handler to parse `waitSeconds`.

- File: `backend/cmd/submission-manager/handlers.go`
- In `handleSubmit`, parse `waitSeconds` from the query string using `r.URL.Query().Get("waitSeconds")`.
- Accept empty or `0` as async (no wait).
- Reject negative or non-integer values with `400 invalid_request`.
- Clamp values greater than the server maximum (default 30 seconds).
- After `SubmitIntent`, if waitSeconds > 0, call the new Manager wait method and return the resulting intent.

2) Add Manager wait method that polls SQL.

- File: `backend/submissionmanager/manager.go`
- Add a public method, for example `WaitForIntent(ctx context.Context, intentID string, wait time.Duration) (Intent, bool, error)`.
- Inside the method, compute a deadline using `m.clock.Now()` and `wait`.
- Loop: load the intent row (status, attempt_count) via `m.store.loadIntentRow` or a small helper method; if terminal status, return; if attempt_count >= 1, return; if timeout elapsed, return the latest state.
- Use `m.clock.After(pollInterval)` between polls. Poll interval default is 250ms.
- If the intent cannot be loaded, return `ok=false` so the handler can map it to `404 not_found` (should not happen after SubmitIntent but keep it safe).

3) Wire waiting into the HTTP handler.

- File: `backend/cmd/submission-manager/handlers.go`
- After `SubmitIntent` succeeds, if waitSeconds > 0, call `manager.WaitForIntent` and return its state. Keep the response shape unchanged.

4) Update specs.

- File: `specs/submission-manager.md`
  - In the HTTP API section for `POST /v1/intents`, note the optional `waitSeconds` query parameter and reference `specs/manager-sync-timeout.md` for full semantics.
- Confirm `specs/manager-sync-timeout.md` remains the canonical detailed description and keep it unchanged unless clarifications are needed.

5) Add tests.

- File: `backend/cmd/submission-manager/main_test.go`
- Add tests for waitSeconds validation (non-integer and negative).
- Add a timeout test: submit with waitSeconds=1 and no manager.Run loop; expect status `pending` and a response within ~1s.
- Add an early-return test: start `manager.Run` in a goroutine with a cancellable context, submit with waitSeconds=5, and assert the response status is not `pending` once the first attempt finishes. Cancel the run loop at the end.

## Validation and Acceptance

Run tests from `backend/`:

- `go test ./cmd/submission-manager -count=1`

Expected result: tests pass. The new waitSeconds tests should fail before the change and pass after.

Manual acceptance (optional):

- Start submission-manager and gateways via `docker compose up -d`.
- Send a request with a wait:

  `curl -sS -X POST 'http://localhost:8082/v1/intents?waitSeconds=10' -H 'Content-Type: application/json' -d '{"intentId":"intent-sync-1","submissionTarget":"sms.realtime","payload":{"to":"+15551234567","message":"hello"}}'`

- Observe that the response returns early (typically within a few seconds) with `status` set to `accepted`, `rejected`, or `exhausted` when the first attempt completes. With `waitSeconds=0`, it returns immediately as `pending`.

## Idempotence and Recovery

All steps are safe to re-run. If tests fail due to missing SQL Server, ensure Docker Compose is running and `MSSQL_SA_PASSWORD` is set (tests skip if not configured). If a test DB remains, re-run the tests; the teardown already drops databases on success or failure.

## Artifacts and Notes

Example successful test output:

  ok   gateway/cmd/submission-manager  (time)

If a validation test fails, confirm that the HTTP handler returns `400` with error code `invalid_request` when `waitSeconds` is negative or non-integer.

## Interfaces and Dependencies

No new dependencies. Use existing packages only.

Add or update the following interfaces and signatures:

- In `backend/submissionmanager/manager.go`, define:

    func (m *Manager) WaitForIntent(ctx context.Context, intentID string, wait time.Duration) (Intent, bool, error)

- If needed, in `backend/submissionmanager/store.go` add a small helper to return `Intent` and `attemptCount` using existing `loadIntentRow` to avoid loading attempts.

- In `backend/cmd/submission-manager/handlers.go`, parse `waitSeconds` and call `Manager.WaitForIntent` when `waitSeconds > 0`.

At the end of the implementation, update this execplan’s `Progress`, `Decision Log`, `Surprises & Discoveries`, and `Outcomes & Retrospective` sections to reflect the work done and any changes in approach.

Plan update note: Marked all work complete, recorded test run, and added outcomes summary after implementation.
