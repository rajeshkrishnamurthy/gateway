# Delivery Tracking V1 Webhook Ingestion Contract-Closure Update

This execplan is a living document. The sections `Progress`, `Surprises & Discoveries`, `Decision Log`, and `Outcomes & Retrospective` must be kept up to date as work proceeds.

This document is maintained in accordance with `backend/PLANS.md`.

EXECPLAN-READY

## Purpose / Big Picture

After this change, a valid `provider signal webhook` request no longer stops at ingress persistence plus handoff-table writes. Instead, the runtime now performs a two-boundary synchronous pipeline: it first commits ingress-audit persistence, then immediately runs correlation plus core-processing apply in the same HTTP request path. This makes webhook ingress contract-closed: accepted writes can now drive canonical delivery read visibility without any persisted queue/handoff stage.

A novice can verify this by posting one `in_progress` webhook for an existing intent and then reading `/v1/intents/{intentId}/delivery` to see `deliveryStatus=in_progress` and `deliveryFreshness=fresh`.

## Progress

- [x] (2026-02-11 16:48Z) Re-read EXEC constraints and execution discipline (`AGENTS.md`, `agents/EXEC.md`, `backend/AGENTS.md`, `backend/cmd/AGENTS.md`, `backend/PLANS.md`).
- [x] (2026-02-11 16:55Z) Audited current webhook/runtime flow and confirmed pre-change behavior persisted ingress plus correlation-handoff rows but did not synchronously invoke core apply from webhook route.
- [x] (2026-02-11 17:08Z) Refactored `backend/deliverytracking/webhook_ingestion.go` to implement Boundary A only: validate/normalize/persist ingress-audit row and commit.
- [x] (2026-02-11 17:12Z) Added deterministic correlation lookup by canonical `intentId` in `backend/deliverytracking/processor.go` for runtime orchestration.
- [x] (2026-02-11 17:16Z) Wired synchronous correlation/core apply in `backend/cmd/delivery-tracking/handlers.go` and `backend/cmd/delivery-tracking/main.go`.
- [x] (2026-02-11 17:20Z) Updated schema contract in `backend/conf/sql/submissionmanager/001_create_schema.sql` to remove creation of persisted correlation-handoff table for new databases.
- [x] (2026-02-11 17:26Z) Updated ingestion and runtime tests for new boundary semantics and added end-to-end webhook->read visibility coverage.
- [x] (2026-02-11 17:31Z) Ran package tests (`go test ./deliverytracking ./cmd/delivery-tracking` and `go test ./...`) and captured passing evidence.
- [x] (2026-02-11 18:00Z) Fixed duplicate `core_processing` failure accounting by removing handler-level metric/log emission for downstream apply errors and relying on processor-level failure accounting.

## Surprises & Discoveries

- Observation: Existing `deliverytracking.Processor` already contained all deterministic core-processing semantics needed for synchronous webhook orchestration; only correlation classification and routing glue were missing.
  Evidence: `backend/deliverytracking/processor.go` already had complete matched/unmatched/invalid no-op and atomic mutation handling.

- Observation: The previous webhook runtime tests asserted `correlation_handoff` stage metrics and no-ingress rollback semantics on downstream failures, which no longer matched the two-boundary model.
  Evidence: `backend/cmd/delivery-tracking/main_test.go` pre-change `TestDeliveryRuntimeWebhookTransientFailureMappedTo503AndNoPartialCommit` dropped `intent_delivery_correlation_handoff` and expected zero ingress rows.

- Observation: Downstream apply failures were counted twice because both handler and processor emitted `core_processing` failure metrics/logs on the same error.
  Evidence: `backend/cmd/delivery-tracking/handlers.go` (apply error path) and `backend/deliverytracking/processor.go` deferred error handler both called `ObserveProcessingFailure`/failure log paths.

## Decision Log

- Decision: Keep webhook ingress persistence and downstream apply as two explicit boundaries in one request path, rather than reintroducing one large transaction across ingress and domain apply.
  Rationale: This directly implements the updated webhook design: ingress-audit evidence must remain durable even when downstream apply fails.
  Date/Author: 2026-02-11 / Codex

- Decision: Classify webhook correlation result via direct `intentId` existence lookup (`matched`/`unmatched`/`invalid`) before invoking `ApplyCorrelatedDeliveryRecord`.
  Rationale: This preserves canonical correlation semantics and prevents unknown intents from surfacing as internal errors.
  Date/Author: 2026-02-11 / Codex

- Decision: Remove schema creation for `intent_delivery_correlation_handoff` in the bootstrap migration file while retaining backward compatibility for already-provisioned databases.
  Rationale: V1 now has no persisted handoff stage; new environments should not create an unused handoff artifact.
  Date/Author: 2026-02-11 / Codex

- Decision: Keep `core_processing` failure accounting single-sourced in processor apply path; remove duplicate handler-side accounting on `ApplyCorrelatedDeliveryRecord` errors.
  Rationale: Prevents double-counted `delivery_processing_failures_total` and duplicate failure logs for one downstream error event.
  Date/Author: 2026-02-11 / Codex

## Outcomes & Retrospective

Implemented outcomes:

- Webhook ingestion now owns only Boundary A persistence in `backend/deliverytracking/webhook_ingestion.go`.
- Runtime now synchronously executes correlation plus core apply after successful ingress commit in `backend/cmd/delivery-tracking/handlers.go`.
- Downstream processing failures now return `503` while preserving ingress-audit rows.
- Downstream apply failures now increment `core_processing` failure metrics exactly once per failing request path.
- Unmatched `intentId` webhook requests are accepted (`202`), persisted as ingress evidence, and remain no-op for delivery state/history mutation.
- End-to-end contract closure is now demonstrated in tests: accepted webhook input updates canonical read API outcomes.

Remaining gap:

- Frozen specs in this checkout still use older handoff wording; this implementation follows the approved design-change direction and keeps behavior deterministic, but any spec text harmonization should be handled in a SPEC session.

## Context and Orientation

Webhook entrypoint is `POST /v1/delivery/provider-signal-webhook` in `backend/cmd/delivery-tracking/routes.go` and `backend/cmd/delivery-tracking/handlers.go`.

Boundary A ingestion logic is in `backend/deliverytracking/webhook_ingestion.go`. It validates required fields, handles malformed optional `providerObservedAt` by omission, gets DB UTC time, generates `sourceRecordId`, and persists one row into `dbo.intent_delivery_webhook_ingestion`.

Downstream domain apply is implemented in `backend/deliverytracking/processor.go`. It already enforces deterministic correlation gating, mode-off no-op, idempotent history writes, terminal lock behavior, and freshness semantics.

Canonical read endpoints are `/v1/intents/{intentId}/delivery` and `/v1/intents/{intentId}/delivery/history` served from `backend/cmd/delivery-tracking/handlers.go` through `backend/deliverytracking/reader.go`.

## Plan of Work

The implementation sequence is:

First, reduce `WebhookIngestor` responsibilities to ingress-audit persistence only and remove persisted correlation-handoff writes.

Second, add explicit correlation classification by canonical key (`intentId`) in the processor layer so the webhook path can choose `matched` vs `unmatched` deterministically.

Third, wire synchronous orchestration in the HTTP handler: ingest, classify correlation, apply core processing, then respond.

Fourth, update tests to reflect two-boundary failure semantics and to prove contract closure from webhook write to delivery read.

Finally, run focused and broad tests and record evidence.

## Concrete Steps

From repository root (`/Users/rajeshk/.codex/worktrees/47cb/setu`):

1. Edit `backend/deliverytracking/webhook_ingestion.go` to remove handoff persistence and keep only ingress write transaction.
2. Edit `backend/deliverytracking/processor.go` to add `CorrelateByIntentID`.
3. Edit `backend/cmd/delivery-tracking/main.go` and `backend/cmd/delivery-tracking/handlers.go` to inject processor and execute synchronous downstream apply.
4. Edit `backend/conf/sql/submissionmanager/001_create_schema.sql` to remove new-database creation of `intent_delivery_correlation_handoff`.
5. Update tests in:
   - `backend/deliverytracking/webhook_ingestion_test.go`
   - `backend/deliverytracking/processor_test.go`
   - `backend/cmd/delivery-tracking/main_test.go`
6. Run from `/Users/rajeshk/.codex/worktrees/47cb/setu/backend`:
   `go test ./deliverytracking -run 'TestWebhookIngest|TestCorrelateByIntentID'`
7. Run:
   `go test ./cmd/delivery-tracking -run 'TestDeliveryRuntimeWebhook'`
8. Run:
   `go test ./deliverytracking ./cmd/delivery-tracking`
9. Run:
   `go test ./...`

## Validation and Acceptance

Acceptance is demonstrated when all of the following are true:

- Invalid webhook payloads still return `400` and produce no ingress row.
- Malformed optional `providerObservedAt` is tolerated and falls back to `receivedAt` effective time.
- A valid webhook for an existing intent returns `202`, persists one ingress row, and changes canonical read visibility (`delivery` and `delivery/history`).
- A valid webhook for an unknown intent returns `202`, persists ingress evidence, and does not mutate delivery state/history.
- Downstream core-processing failure after ingress commit returns `503` while keeping the ingress row persisted.

## Idempotence and Recovery

Schema and code changes are safe to rerun in a fresh workspace. Runtime behavior remains deterministic under duplicate webhook requests because each accepted ingress receives a unique `sourceRecordId`, while downstream domain idempotency is enforced by `(intentId, sourceRecordId)` in delivery history.

If downstream apply fails transiently, retrying the webhook request creates a new ingress row and a new downstream attempt, which is expected for this V1 boundary model.

## Artifacts and Notes

Validation evidence from `/Users/rajeshk/.codex/worktrees/47cb/setu/backend`:

  $ go test ./deliverytracking -run 'TestWebhookIngest|TestCorrelateByIntentID'
  ok   gateway/deliverytracking  0.590s

  $ go test ./cmd/delivery-tracking -run 'TestDeliveryRuntimeWebhook'
  ok   gateway/cmd/delivery-tracking  1.015s

  $ go test ./deliverytracking ./cmd/delivery-tracking
  ok   gateway/deliverytracking       0.388s
  ok   gateway/cmd/delivery-tracking  0.217s

  $ go test ./...
  ok   gateway/... (all packages passed)

## Interfaces and Dependencies

No new external dependencies were added.

Key interfaces after this implementation:

- `deliverytracking.WebhookIngestor.IngestProviderSignalWebhook(ctx, rawPayload)`
  returns normalized ingress result after ingress-audit persistence commit.

- `deliverytracking.Processor.CorrelateByIntentID(ctx, intentID)`
  classifies canonical correlation result as `matched`, `unmatched`, or `invalid`.

- `deliverytracking.Processor.ApplyCorrelatedDeliveryRecord(ctx, record)`
  applies deterministic core-processing semantics and no-op gating using the classified correlation result.

---

Revision note (2026-02-11 / Codex): Rewrote this execplan to match the approved two-boundary webhook design and to capture implementation evidence for synchronous contract closure from webhook ingress to canonical read behavior.
Revision note (2026-02-11 / Codex): Updated plan after fixing duplicate `core_processing` failure accounting between webhook handler and processor apply path.
