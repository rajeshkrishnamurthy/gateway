# Delivery Tracking V1 Webhook Ingestion Implementation

This execplan is a living document. The sections `Progress`, `Surprises & Discoveries`, `Decision Log`, and `Outcomes & Retrospective` must be kept up to date as work proceeds.

This document is maintained in accordance with `/Users/rajeshk/code/go/go-toolkit/setu/backend/PLANS.md`.

EXECPLAN-READY

## Purpose / Big Picture

After this change, Setu will accept trusted-ingress provider delivery webhook payloads, normalize them deterministically, persist normalized ingestion records, and perform deterministic correlation handoff in the same SQL transaction. Invalid normalized payloads will return `400`-class responses, and transient internal failures will return `503`-class responses. This allows the delivery-tracking path to start from a stable webhook-ingestion boundary without introducing async queue workers or security hardening in this round.

A novice can verify the result by running targeted tests in `/Users/rajeshk/code/go/go-toolkit/setu/backend/deliverytracking` and `/Users/rajeshk/code/go/go-toolkit/setu/backend/cmd/submission-manager` that demonstrate validation behavior, malformed `providerObservedAt` fallback, timestamp precedence propagation, duplicate/replay acceptance, and transaction rollback on handoff failure.

## Progress

- [x] (2026-02-11 03:26Z) Read all required EXEC inputs in the mandated order for webhook-ingestion slice.
- [x] (2026-02-11 03:26Z) Mapped current code boundaries: HTTP adapter (`cmd/submission-manager`) and delivery domain core (`backend/deliverytracking`).
- [x] (2026-02-11 03:31Z) Implemented delivery webhook-ingestion core in `/Users/rajeshk/code/go/go-toolkit/setu/backend/deliverytracking/webhook_ingestion.go` with validation, normalization, timestamp precedence, atomic persistence, and typed invalid-payload errors.
- [x] (2026-02-11 03:31Z) Added schema support in `/Users/rajeshk/code/go/go-toolkit/setu/backend/conf/sql/submissionmanager/001_create_schema.sql` for ingestion and correlation-handoff tables plus uniqueness constraints.
- [x] (2026-02-11 03:31Z) Wired HTTP endpoint `/v1/delivery/provider-signal-webhook` in `/Users/rajeshk/code/go/go-toolkit/setu/backend/cmd/submission-manager` with deterministic `400` vs `503` mapping.
- [x] (2026-02-11 03:32Z) Added targeted semantic tests in `/Users/rajeshk/code/go/go-toolkit/setu/backend/deliverytracking/webhook_ingestion_test.go` and `/Users/rajeshk/code/go/go-toolkit/setu/backend/cmd/submission-manager/main_test.go`.
- [x] (2026-02-11 03:33Z) Ran focused feature tests and broader package tests; captured passing evidence and isolated unrelated pre-existing failure evidence.
- [x] (2026-02-11 03:33Z) Finalized outcomes and blockers in this execplan.

## Surprises & Discoveries

- Observation: No existing delivery webhook-ingestion endpoint or ingestion persistence model is present; current HTTP service only exposes intent routes.
  Evidence: `/Users/rajeshk/code/go/go-toolkit/setu/backend/cmd/submission-manager/routes.go` currently registers `/v1/intents` and `/v1/intents/` only.
- Observation: `go test` commands must run from module root `/Users/rajeshk/code/go/go-toolkit/setu/backend`; running from repository root fails with module discovery error.
  Evidence: `go: cannot find main module, but found .git/config in /Users/rajeshk/code/go/go-toolkit/setu`.
- Observation: A broader submission-manager test fails outside this slice and remains unrelated to webhook-ingestion behavior.
  Evidence: `go test ./deliverytracking ./cmd/submission-manager` fails on `TestSubmitWaitSecondsEarlyReturn` (`expected early return, elapsed ~5.01s`), while all `TestDeliveryWebhook*` tests pass.

## Decision Log

- Decision: Keep webhook-ingestion domain logic in `backend/deliverytracking` and keep `cmd/submission-manager` as a pure HTTP adapter.
  Rationale: Matches explicit context separation between submission and delivery domains and complies with `backend/cmd/AGENTS.md` handler boundary rules.
  Date/Author: 2026-02-11 / Codex
- Decision: For accepted payloads, generate `receivedAt` using `SYSUTCDATETIME()` inside the same transaction and derive `effectiveAt` as `providerObservedAt` when valid, else `receivedAt`.
  Rationale: Enforces design timestamp precedence and keeps persisted times deterministic across multi-instance ingestion.
  Date/Author: 2026-02-11 / Codex
- Decision: Do not perform ingestion-level duplicate suppression; each valid payload instance receives a new generated `sourceRecordId` and is persisted.
  Rationale: Frozen design explicitly requires duplicate/replay acceptance at ingestion.
  Date/Author: 2026-02-11 / Codex
- Decision: Map `InvalidWebhookPayloadError` to `400 invalid_request`; map all other ingestion failures to `503 service_unavailable`.
  Rationale: Preserves deterministic boundary classification required by design and avoids leaking internal persistence errors as client faults.
  Date/Author: 2026-02-11 / Codex

## Outcomes & Retrospective

Webhook-ingestion slice implementation is complete for trusted ingress scope. The service now accepts provider webhook payloads, validates and normalizes required fields, tolerates malformed `providerObservedAt` by omitting it, persists ingestion and correlation-handoff records atomically, and returns deterministic HTTP classes (`400` invalid payload, `503` transient internal failure, `202` accepted for valid ingress). This behavior is implemented in `/Users/rajeshk/code/go/go-toolkit/setu/backend/deliverytracking/webhook_ingestion.go` and wired through `/Users/rajeshk/code/go/go-toolkit/setu/backend/cmd/submission-manager/handlers.go`.

All semantic-critical tests for this slice pass. A non-slice test failure (`TestSubmitWaitSecondsEarlyReturn`) remains in broader `cmd/submission-manager` runs and is documented as unrelated evidence rather than changed behavior in this work.

## Context and Orientation

The existing delivery core-processing implementation remains in `/Users/rajeshk/code/go/go-toolkit/setu/backend/deliverytracking/processor.go` and still applies normalized correlated signals transactionally. This webhook-ingestion slice adds a separate ingress stage in `/Users/rajeshk/code/go/go-toolkit/setu/backend/deliverytracking/webhook_ingestion.go`. The submission-manager HTTP service remains the adapter boundary in `/Users/rajeshk/code/go/go-toolkit/setu/backend/cmd/submission-manager`, with route registration in `routes.go`, request adapter logic in `handlers.go`, and response shaping in `responses.go`.

Webhook-ingestion for this slice must stop at normalized persistence plus correlation handoff. It must not implement delivery status progression logic, read APIs, provider polling, or security hardening controls. Trusted-ingress-only behavior must remain explicit.

In this plan, `provider signal webhook` means provider-to-Setu push ingress. `correlation handoff` means durable transfer of normalized fields to the next deterministic stage in the same transaction boundary, not asynchronous queue-worker processing.

## Plan of Work

Implementation followed this sequence. First, `/Users/rajeshk/code/go/go-toolkit/setu/backend/deliverytracking/webhook_ingestion.go` introduced `WebhookIngestor` with payload normalization, required field validation, optional malformed `providerObservedAt` fallback-to-omit behavior, deterministic signal-class parsing, and transactional persistence/handoff with rollback semantics.

Second, `/Users/rajeshk/code/go/go-toolkit/setu/backend/conf/sql/submissionmanager/001_create_schema.sql` added `intent_delivery_webhook_ingestion` and `intent_delivery_correlation_handoff` with source-record uniqueness indexes and idempotent creation guards.

Third, `/Users/rajeshk/code/go/go-toolkit/setu/backend/cmd/submission-manager/routes.go`, `/Users/rajeshk/code/go/go-toolkit/setu/backend/cmd/submission-manager/handlers.go`, `/Users/rajeshk/code/go/go-toolkit/setu/backend/cmd/submission-manager/responses.go`, and `/Users/rajeshk/code/go/go-toolkit/setu/backend/cmd/submission-manager/main.go` wired the endpoint and mapped invalid payloads to `400` and transient internal failures to `503`.

Fourth, targeted tests were added in `/Users/rajeshk/code/go/go-toolkit/setu/backend/deliverytracking/webhook_ingestion_test.go` and `/Users/rajeshk/code/go/go-toolkit/setu/backend/cmd/submission-manager/main_test.go` for semantic-critical behaviors from the frozen design.

Finally, focused tests and broader package runs were executed and captured below.

## Concrete Steps

From `/Users/rajeshk/code/go/go-toolkit/setu`:

1. Add ingestion core files and tests under `backend/deliverytracking`.
2. Update SQL schema in `backend/conf/sql/submissionmanager/001_create_schema.sql`.
3. Add HTTP route and handler wiring in `backend/cmd/submission-manager`.
4. Change working directory to `/Users/rajeshk/code/go/go-toolkit/setu/backend` (Go module root).
5. Run:
   `go test ./deliverytracking -run 'TestWebhookIngest'`
6. Run:
   `go test ./cmd/submission-manager -run 'TestDeliveryWebhook'`
7. Run:
   `go test ./deliverytracking ./cmd/submission-manager`
8. Isolate unrelated broader-suite failure evidence:
   `go test ./cmd/submission-manager -run '^TestSubmitWaitSecondsEarlyReturn$' -count=1`

Expected outcome: webhook-ingestion tests pass and prove deterministic behavior per frozen design; one unrelated pre-existing `TestSubmitWaitSecondsEarlyReturn` failure may still appear in broader submission-manager runs.

## Validation and Acceptance

Acceptance is demonstrated by tests that prove:

- required field validation rejects invalid payloads (`400` class at HTTP adapter).
- malformed `providerObservedAt` is accepted and omitted from normalized record/handoff.
- normalization output stores UTC timestamps.
- effective time precedence uses valid `providerObservedAt`, otherwise `receivedAt`.
- ingestion persistence and correlation handoff are atomic in one transaction; no partial commit on failure.
- duplicate/replayed valid payloads are accepted (no ingestion-level suppression).
- transient internal failures map to `503` class at HTTP adapter.

## Idempotence and Recovery

Schema changes are idempotent via `IF OBJECT_ID`/`IF NOT EXISTS` guards. Ingestion intentionally does not deduplicate payloads, so repeated valid requests produce additional accepted records by design. If handoff fails, transaction rollback leaves no partial ingestion record for that request.

## Artifacts and Notes

Test evidence from `/Users/rajeshk/code/go/go-toolkit/setu/backend`:

    $ go test ./deliverytracking -run 'TestWebhookIngest'
    ok  	gateway/deliverytracking	(cached)

    $ go test ./cmd/submission-manager -run 'TestDeliveryWebhook'
    ok  	gateway/cmd/submission-manager	(cached)

    $ go test ./deliverytracking ./cmd/submission-manager
    ok  	gateway/deliverytracking	(cached)
    --- FAIL: TestSubmitWaitSecondsEarlyReturn (5.28s)
        main_test.go:253: expected early return, elapsed 5.010023209s
    FAIL
    FAIL	gateway/cmd/submission-manager	9.765s
    FAIL

    $ go test ./cmd/submission-manager -run '^TestSubmitWaitSecondsEarlyReturn$' -count=1
    --- FAIL: TestSubmitWaitSecondsEarlyReturn (5.35s)
        main_test.go:253: expected early return, elapsed 5.012374875s
    FAIL
    FAIL	gateway/cmd/submission-manager	5.570s
    FAIL

## Interfaces and Dependencies

No new external dependencies are introduced.

Implemented core interface and types in `/Users/rajeshk/code/go/go-toolkit/setu/backend/deliverytracking`:

- `WebhookIngestor` with method:
  - `IngestProviderSignalWebhook(ctx context.Context, rawPayload []byte) (WebhookIngestionResult, error)`
- `InvalidWebhookPayloadError` to distinguish `400` class failures from transient `503` class failures.
- `WebhookIngestionResult` carrying normalized handoff fields (`intentId`, `providerDeliverySignal`, `providerObservedAt` optional, `receivedAt`, source metadata, generated `sourceRecordId`, and effective-time propagation field).

`/Users/rajeshk/code/go/go-toolkit/setu/backend/cmd/submission-manager` now calls this core directly and only performs HTTP parsing plus response/error mapping for `/v1/delivery/provider-signal-webhook`.

---

Revision note (2026-02-11 / Codex): Initial webhook-ingestion execplan created after mandated read order and boundary reconnaissance.
Revision note (2026-02-11 / Codex): Updated living sections after implementation, recorded deterministic design decisions and concrete test evidence, and documented unrelated broader-suite failure isolation.
