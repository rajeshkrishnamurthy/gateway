# Fix waitSeconds early return regression in submission-manager tests

This execplan is a living document. The sections `Progress`, `Surprises & Discoveries`, `Decision Log`, and `Outcomes & Retrospective` must be kept up to date as work proceeds.

This plan must be maintained in accordance with `backend/PLANS.md` from the repository root.

EXECPLAN-READY

## Purpose / Big Picture

After this slice, `POST /v1/intents?waitSeconds=N` test coverage will again prove the intended contract: the wait path returns as soon as the first attempt completes, even when intent status remains `pending`. The regression happened after leader-lease gating because the test started `Manager.Run` directly without establishing leadership, so no attempt executed and the call waited until timeout. The fix keeps leader-lease semantics intact by making the test run through lease-aware leadership startup.

## Progress

- [x] (2026-02-11 05:37Z) Read required EXEC/session documents and specs in the requested order, including leader-lease and waitSeconds specs plus `backend/PLANS.md`.
- [x] (2026-02-11 05:37Z) Inspected current `waitSeconds` implementation and `TestSubmitWaitSecondsEarlyReturn` behavior in `backend/cmd/submission-manager/main_test.go`.
- [x] (2026-02-11 05:37Z) Identified regression root cause: `manager.Run` now requires explicit leadership; test path does not acquire lease.
- [x] (2026-02-11 05:41Z) Implemented focused test/runtime harness fix in `backend/cmd/submission-manager/main_test.go` by starting a `LeaderRunner` and waiting for leadership before submit in `TestSubmitWaitSecondsEarlyReturn`.
- [x] (2026-02-11 05:42Z) Ran and recorded required command evidence for `go test -count=1 ./cmd/submission-manager` plus focused wait tests in `./submissionmanager`.
- [x] (2026-02-11 05:43Z) Updated this execplan living sections with final outcomes and evidence.

## Surprises & Discoveries

- Observation: In this environment, SQL-backed tests skip when `MSSQL_SA_PASSWORD` is not configured, so direct runtime reproduction of the reported failure is blocked locally.
  Evidence: `go test -count=1 ./cmd/submission-manager -run TestSubmitWaitSecondsEarlyReturn -v` reports `SKIP` with message `MSSQL_SA_PASSWORD not set; start docker compose and set env or backend/.env`.
- Observation: The required and focused test commands still provide compile/runtime harness validation even when SQL-backed cases are skipped.
  Evidence: `go test -count=1 ./cmd/submission-manager` and `go test -count=1 ./submissionmanager -run 'TestWaitForIntent(Missing|AfterFirstAttempt)' -v` both return `PASS` with explicit skip messages tied only to missing SQL password configuration.

## Decision Log

- Decision: Fix the regression by updating test startup to use `LeaderRunner` and explicit leadership wait, not by weakening leader checks in production scheduler code.
  Rationale: This keeps leader/follower and lease fencing semantics unchanged while restoring deterministic early-return behavior in the waitSeconds contract test.
  Date/Author: 2026-02-11 / Codex

- Decision: Keep timeout and invalid-input waitSeconds tests behavior unchanged and only tighten leadership assumptions in the early-return test path.
  Rationale: Requested scope is narrow to wait early-return semantics; timeout and validation are separate contract checks that should remain stable.
  Date/Author: 2026-02-11 / Codex

- Decision: Wait for `runner.IsLeader()` before issuing the test submission request.
  Rationale: This removes startup timing races between lease acquisition and intent submit, ensuring deterministic first-attempt execution in the early-return assertion.
  Date/Author: 2026-02-11 / Codex

## Outcomes & Retrospective

The regression fix was completed by aligning the early-return integration test with leader-lease runtime semantics. The test now starts a lease-aware runner and blocks until leadership is established, so attempt execution can occur and the wait path can return early on first attempt completion as intended. Timeout and invalid waitSeconds tests were left unchanged. No delivery-tracking topology/runtime paths were modified.

The only validation gap is environmental: SQL-backed test cases are skipped in this workspace because `MSSQL_SA_PASSWORD` is not configured. Command-level evidence still confirms package compilation and test harness integrity after the change.

## Context and Orientation

Relevant files and behavior:

- `backend/submissionmanager/schedule.go`: `Manager.Run` exits immediately when not leader.
- `backend/submissionmanager/leader_runner.go`: lease-aware startup, renew, refresh, and leadership transitions.
- `backend/cmd/submission-manager/main_test.go`: `TestSubmitWaitSecondsEarlyReturn` currently calls `go manager.Run(ctx)` directly.
- `backend/submissionmanager/manager.go`: `WaitForIntent` returns early when first attempt completes (`attempt_count >= 1`) or when terminal.

Contract constraints:

- `specs/manager-sync-timeout.md`: wait ends on terminal state, first attempt completion, or timeout.
- `specs/submission-tracking/submission-manager-leaderlease.md`: only leader executes attempts; follower must not execute.

Because attempt execution is now leader-gated, test setup must establish leadership before expecting first-attempt completion timing.

## Plan of Work

Update `backend/cmd/submission-manager/main_test.go` to start a lease-aware runner in the early-return test and wait until that runner is leader before issuing the submit request. Use short lease timings so tests are fast and deterministic. Keep the rest of the waitSeconds tests unchanged except for any helper extraction needed for readability.

If required for focused coverage, add or adjust a narrow test in `backend/submissionmanager` that asserts `WaitForIntent` returns after first attempt under active leadership assumptions (without changing runtime semantics).

## Concrete Steps

From working directory `/Users/rajeshk/.codex/worktrees/0141/setu/backend`:

1. Run targeted package tests while iterating:

   `go test -count=1 ./cmd/submission-manager -run 'TestSubmitWaitSeconds(EarlyReturn|Timeout|Invalid|Negative)'`

2. Run required command:

   `go test -count=1 ./cmd/submission-manager`

3. Run focused submissionmanager tests if touched:

   `go test -count=1 ./submissionmanager -run 'TestWaitForIntent(Missing|AfterFirstAttempt)'`

Record concrete outcomes in `Artifacts and Notes`.

## Validation and Acceptance

Acceptance for this slice:

- `TestSubmitWaitSecondsEarlyReturn` returns in well under the full wait window when first attempt completes and still returns `pending` when non-terminal.
- `TestSubmitWaitSecondsTimeout` still waits approximately timeout duration when no attempt completion is observed.
- Invalid waitSeconds tests remain unchanged (`400 invalid_request` behavior).
- No delivery-tracking runtime/topology changes are introduced.
- Leader-lease semantics remain intact: tests rely on leadership acquisition rather than bypassing leader checks.

## Idempotence and Recovery

Edits are limited to waitSeconds regression scope and are safe to re-run. If SQL-backed tests are skipped due missing env, document skip evidence and keep command transcripts; no destructive recovery steps are required.

## Artifacts and Notes

Initial evidence (pre-fix context):

`go test -count=1 ./cmd/submission-manager -run TestSubmitWaitSecondsEarlyReturn -v`

produces:

`SKIP: MSSQL_SA_PASSWORD not set; start docker compose and set env or backend/.env`

Post-fix command evidence:

`go test -count=1 ./cmd/submission-manager -run 'TestSubmitWaitSeconds(EarlyReturn|Timeout|Invalid|Negative)' -v`

- `PASS`
- `TestSubmitWaitSecondsInvalid`: `SKIP` (missing `MSSQL_SA_PASSWORD`)
- `TestSubmitWaitSecondsNegative`: `SKIP` (missing `MSSQL_SA_PASSWORD`)
- `TestSubmitWaitSecondsTimeout`: `SKIP` (missing `MSSQL_SA_PASSWORD`)
- `TestSubmitWaitSecondsEarlyReturn`: `SKIP` (missing `MSSQL_SA_PASSWORD`)

`go test -count=1 ./cmd/submission-manager`

- `ok   gateway/cmd/submission-manager`

`go test -count=1 ./submissionmanager -run 'TestWaitForIntent(Missing|AfterFirstAttempt)' -v`

- `PASS`
- `TestWaitForIntentMissing`: `SKIP` (missing `MSSQL_SA_PASSWORD`)
- `TestWaitForIntentAfterFirstAttempt`: `SKIP` (missing `MSSQL_SA_PASSWORD`)

## Interfaces and Dependencies

No new dependencies are expected. Existing types and APIs should remain unchanged:

- `submissionmanager.NewLeaderRunnerFromManager`
- `submissionmanager.LeaseConfig`
- `(*submissionmanager.Manager).WaitForIntent`

Plan update note (2026-02-11): Created this dedicated EXEC execplan with `EXECPLAN-READY`, captured root cause, and constrained work to early-return waitSeconds regression plus directly related tests.
Plan update note (2026-02-11): Marked implementation complete, documented the leader-runner test harness fix, and recorded concrete command evidence plus environment-related skip limitations.
