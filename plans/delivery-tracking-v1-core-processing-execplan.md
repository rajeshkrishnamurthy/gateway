# Delivery Tracking V1 Core Processing Implementation

This execplan is a living document. The sections `Progress`, `Surprises & Discoveries`, `Decision Log`, and `Outcomes & Retrospective` must be kept up to date as work proceeds.

This document is maintained in accordance with `/Users/rajeshk/code/go/go-toolkit/setu/backend/PLANS.md`.

EXECPLAN-READY

## Purpose / Big Picture

After this change, the repository will have a deterministic core-processing path that applies correlated provider delivery signals into durable delivery history plus current delivery state for each intent. This enables reproducible delivery status and freshness behavior under duplicate, out-of-order, and concurrent processing without changing submission status semantics.

A novice can verify this behavior by running targeted `go test` cases in `/Users/rajeshk/code/go/go-toolkit/setu/backend/deliverytracking` and observing passing scenarios for correlation gating, mode-off gating, idempotency, tie-break ordering, stale-to-fresh rules, terminal conflict lock behavior, late terminal observations, and transaction rollback safety.

## Progress

- [x] (2026-02-11 02:12Z) Read required inputs in mandated order: root/EXEC/component AGENTS, umbrella/core/foundational specs, design docs, and `/Users/rajeshk/code/go/go-toolkit/setu/backend/PLANS.md`.
- [x] (2026-02-11 02:12Z) Audited current code and schema to locate integration points for contract snapshot, SQL persistence, and transactional write patterns.
- [x] (2026-02-11 02:21Z) Implemented delivery-tracking contract snapshot plumbing (registry validation + intent persistence fields in `submission_intents`).
- [x] (2026-02-11 02:21Z) Added SQL schema objects for delivery current-state projection and delivery history with required keys/indexes.
- [x] (2026-02-11 02:21Z) Implemented core processing transactional apply path with deterministic ordering, idempotency, terminal lock/conflict semantics, and late-observation annotation.
- [x] (2026-02-11 02:21Z) Added staleness evaluator write path (`fresh -> stale`) while keeping stale-to-fresh only in signal-apply path.
- [x] (2026-02-11 02:21Z) Added targeted tests for semantic-critical cases listed in the EXEC request.
- [x] (2026-02-11 02:21Z) Ran relevant targeted tests and recorded concrete evidence.
- [x] (2026-02-11 02:26Z) Refactored delivery core-processing implementation/tests into `/Users/rajeshk/code/go/go-toolkit/setu/backend/deliverytracking` and kept only thin integration adapters in `submissionmanager`.
- [x] (2026-02-11 02:26Z) Re-ran package validation including full `go test ./submissionmanager`; all selected tests passed.

## Surprises & Discoveries

- Observation: Existing repository code has no delivery-tracking contract fields in `submission.TargetContract` and no delivery-tracking SQL tables yet.
  Evidence: `/Users/rajeshk/code/go/go-toolkit/setu/backend/submission/registry.go` and `/Users/rajeshk/code/go/go-toolkit/setu/backend/conf/sql/submissionmanager/001_create_schema.sql` currently model submission and webhook data only.

- Observation: During refactor, `/Users/rajeshk/code/go/go-toolkit/setu/backend/submissionmanager/leader_runner_test.go` changed concurrently in another Codex session.
  Evidence: `git status --short` showed the file modified outside this feature; user explicitly instructed to keep that unrelated change as-is.

## Decision Log

- Decision: Implement the core-processing entrypoint inside `submissionmanager` as a transactional store-backed method and keep transport/webhook ingestion out of scope.
  Rationale: The frozen core slice starts after correlation and must focus on deterministic apply semantics only.
  Date/Author: 2026-02-11 / Codex

- Decision: Acquire per-intent locks in order `submission_intents` row, then `intent_delivery_state` row/range lock, then history insert and projection mutation inside one transaction.
  Rationale: This matches the design’s lock-order convention while still preserving idempotent duplicate no-op behavior and avoiding pre-creating state rows for duplicate source records.
  Date/Author: 2026-02-11 / Codex

- Decision: Add a test-only `sqlStore` hook immediately after history insert to force rollback-path verification.
  Rationale: This provides deterministic proof that no partial mutation commits when a failure occurs after history insert but before projection update.
  Date/Author: 2026-02-11 / Codex

- Decision: Move delivery core-processing code and tests into `backend/deliverytracking`, leaving `submissionmanager` with a thin adapter only.
  Rationale: This preserves the semantic boundary between submission and delivery contexts while keeping integration behavior unchanged.
  Date/Author: 2026-02-11 / Codex

- Decision: Keep concurrent unrelated edits in `/Users/rajeshk/code/go/go-toolkit/setu/backend/submissionmanager/leader_runner_test.go` untouched.
  Rationale: User confirmed those changes were intentional from a separate session and should not be reverted.
  Date/Author: 2026-02-11 / Codex

## Outcomes & Retrospective

Implemented scope:

- Registry and contract snapshot now support `deliveryTracking.mode` and `deliveryTracking.staleAfterSeconds` with validation rules aligned to foundational specs.
- Intent persistence now stores snapshotted delivery-tracking mode/stale settings.
- SQL schema now includes `intent_delivery_state` and `intent_delivery_history`, idempotency unique index `(intent_id, source_record_id)`, and freshness evaluator index.
- Core-processing write path now lives in `/Users/rajeshk/code/go/go-toolkit/setu/backend/deliverytracking/processor.go` and enforces:
  - `matched` gating,
  - `mode=off` no mutation,
  - atomic history + projection transaction boundary,
  - idempotent duplicate no-op by source record,
  - deterministic ordering key/tie-break semantics,
  - stale-to-fresh only on newer valid `in_progress`,
  - terminal lock with opposite-terminal conflict marker as history-only,
  - late terminal observation annotation.
- Freshness evaluator write path (`fresh -> stale`) is implemented as persisted update logic.
- Targeted tests for all requested semantic-critical behaviors live in `/Users/rajeshk/code/go/go-toolkit/setu/backend/deliverytracking/processor_test.go`.
- `submissionmanager` retains integration-only adapters in `/Users/rajeshk/code/go/go-toolkit/setu/backend/submissionmanager/deliverytracking_adapter.go`.
- Validation commands for `submission`, `deliverytracking`, and full `submissionmanager` package runs are passing in this workspace.

## Context and Orientation

SubmissionManager stores intents in SQL table `dbo.submission_intents` and attempt history in `dbo.submission_attempts`, with explicit per-operation transactions in `/Users/rajeshk/code/go/go-toolkit/setu/backend/submissionmanager/store_attempts.go`. Contract snapshots are loaded from `/Users/rajeshk/code/go/go-toolkit/setu/backend/submission/registry.go`, cloned in `/Users/rajeshk/code/go/go-toolkit/setu/backend/submissionmanager/snapshot.go`, and persisted/loaded in `/Users/rajeshk/code/go/go-toolkit/setu/backend/submissionmanager/store_intents.go`.

Delivery core processing is now implemented in `/Users/rajeshk/code/go/go-toolkit/setu/backend/deliverytracking/processor.go`, with `submissionmanager` calling it through `/Users/rajeshk/code/go/go-toolkit/setu/backend/submissionmanager/deliverytracking_adapter.go`.

The delivery core-processing work adds two delivery-specific persistence surfaces:

1. Current projection row per intent (`intent_delivery_state`) that stores canonical delivery status/freshness and ordering watermarks.
2. Append-semantics history (`intent_delivery_history`) that stores each accepted correlated source record and annotations such as late observation and terminal conflict marker.

In this plan, "ordering key K" means the strict lexicographic tuple `(effectiveAt, receivedAt, sourceRecordId)`, where `effectiveAt` is provider-observed time when valid, otherwise received time. "Terminal lock" means first observed terminal delivery polarity (`delivered` or `failed`) freezes current delivery status while opposite-polarity later terminal signals are history-only.

## Plan of Work

First, extend submission registry contract parsing to include `deliveryTracking` settings with validation rules from frozen specs: mode `on/off`, required positive `staleAfterSeconds` for `on`, and ignored stale value for `off`. Persist these snapshot fields to `submission_intents` so core processing can lock and read authoritative per-intent delivery mode and staleness threshold from the intent snapshot.

Second, extend SQL schema file `/Users/rajeshk/code/go/go-toolkit/setu/backend/conf/sql/submissionmanager/001_create_schema.sql` to add delivery tables and indexes required by the design: `intent_delivery_state`, `intent_delivery_history`, unique idempotency key on `(intent_id, source_record_id)`, and evaluator-supporting index over projection freshness fields.

Third, implement core-processing domain types and write path in `deliverytracking`: correlated record input, deterministic key computation, SQL transaction boundary, correlation/mode gates, idempotent history insertion, projection upsert and transition application, conflict marker update, and late-observation annotation.

Fourth, implement the time-driven staleness evaluator write path that moves unresolved `fresh` rows to `stale` when `stale_at` is due, keeping stale-to-fresh exclusively in signal-apply path.

Fifth, add targeted integration-style tests in `deliverytracking` covering the requested behaviors. Keep tests boring and explicit: seed intents, apply signals, and assert final projection/history rows and no-partial-write rollback on injected mid-transaction failure.

Finally, run focused `go test` commands, capture output snippets, and update all living sections with completed evidence.

## Concrete Steps

From `/Users/rajeshk/code/go/go-toolkit/setu`:

1. Edit registry and submissionmanager persistence code for delivery-tracking snapshot fields.
2. Edit SQL schema file to add new columns/tables/indexes.
3. Add core-processing implementation files under `backend/deliverytracking` and integration adapters in `backend/submissionmanager`.
4. Add targeted tests under `backend/deliverytracking` and, if needed, `backend/submission`.
5. Run:
   `go test ./backend/submission/...`
6. Run:
   `go test ./backend/deliverytracking -run 'TestDelivery'`
7. Run:
   `go test ./backend/submissionmanager`

Expected outcome is all tests passing (or skipped only when MSSQL environment is intentionally unavailable in this workspace).

## Validation and Acceptance

Acceptance for this slice is demonstrated by tests that prove:

- `unmatched` and `invalid` correlation results do not mutate delivery projection/history.
- `mode=off` does not mutate delivery projection/history.
- deterministic ordering and tie-break behavior for equal timestamps converges consistently.
- duplicate `sourceRecordId` apply attempts are idempotent no-ops.
- stale-to-fresh transition occurs only for newer valid `in_progress` signals.
- first terminal polarity locks current status; opposite terminal later is history-only conflict marker.
- terminal signals can be annotated as late observations when submission was already terminal at receive time.
- transaction rollback prevents partial mutation if failure occurs after history insert but before projection mutation.

## Idempotence and Recovery

All schema edits are written in idempotent SQL style (`IF OBJECT_ID`, `IF COL_LENGTH`, `IF NOT EXISTS`) so rerunning the schema is safe. Core apply path is idempotent by `(intentId, sourceRecordId)` and commits history/projection in one transaction to avoid partial state.

If a test fails after schema creation, rerunning `go test` is safe because each integration test creates a fresh temporary database via `newTestDB`.

## Artifacts and Notes

Executed commands and observed outcomes:

  /Users/rajeshk/code/go/go-toolkit/setu/backend$ go test ./submission/...
  ok  	gateway/submission	0.654s

  /Users/rajeshk/code/go/go-toolkit/setu/backend$ go test ./deliverytracking -run 'TestDelivery'
  ok  	gateway/deliverytracking	6.952s

  /Users/rajeshk/code/go/go-toolkit/setu/backend$ go test ./submissionmanager
  ok  	gateway/submissionmanager	18.085s

## Interfaces and Dependencies

No new third-party dependencies will be added. The implementation remains in stdlib plus existing SQL Server driver usage.

Implemented interfaces/types in `deliverytracking`:

- A correlated delivery input type carrying:
  - `sourceRecordId`
  - `providerEventId` (optional)
  - `intentId`
  - normalized signal class (`in_progress`, `terminal_success`, `terminal_failure`)
  - `providerObservedAt` (optional)
  - `receivedAt`
  - `correlationResult` (`matched`, `unmatched`, `invalid`)
- A transactional apply method returning explicit result classification (history inserted/state mutated/no-op reason/late/conflict markers).
- A staleness evaluator method that updates due unresolved rows from `fresh` to `stale`.

Integration in `submissionmanager` is intentionally thin and delegates to `deliverytracking.Processor`.

Implementation note: if low-level lock sequencing is ambiguous between two equivalent deterministic SQL patterns, the chosen pattern will be recorded here in `Decision Log` with rationale, provided no user-visible semantics change.

---

Revision note (2026-02-11 / Codex): Initial execplan creation after mandatory input review and codebase reconnaissance.
Revision note (2026-02-11 / Codex): Updated plan after implementation and testing; recorded lock-order decision, rollback-test hook decision, and current full-package test gap.
Revision note (2026-02-11 / Codex): Refactored delivery core to `backend/deliverytracking` to preserve bounded-context semantics; updated validation evidence with passing package runs.
