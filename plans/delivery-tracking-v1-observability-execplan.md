# Delivery Tracking V1 Observability Implementation

This execplan is a living document. The sections `Progress`, `Surprises & Discoveries`, `Decision Log`, and `Outcomes & Retrospective` must be kept up to date as work proceeds.

This document is maintained in accordance with `/Users/rajeshk/code/go/go-toolkit/setu/backend/PLANS.md`.

EXECPLAN-READY

## Purpose / Big Picture

After this change, the delivery-tracking runtime will expose deterministic delivery observability on its existing metrics surface and structured decision/failure logs, without changing delivery domain semantics. Operators will be able to inspect ingestion outcomes, correlation/apply no-op reasons, committed delivery/freshness transitions, read API request codes, and canonical current delivery-status counts from durable state.

A novice can verify behavior by running targeted tests in `/Users/rajeshk/code/go/go-toolkit/setu/backend/deliverytracking` and `/Users/rajeshk/code/go/go-toolkit/setu/backend/cmd/delivery-tracking`, then scraping `/metrics` from the delivery-tracking runtime handlers in tests.

## Progress

- [x] (2026-02-11 07:35Z) Read required governance and slice inputs (`AGENTS`, `EXEC`, backend/component AGENTS, observability spec/design, `backend/PLANS.md`).
- [x] (2026-02-11 07:35Z) Mapped concrete instrumentation points in webhook ingestion handlers, delivery core processing (`processor.go`), freshness evaluator, read API handlers, and delivery runtime route wiring.
- [x] (2026-02-11 07:43Z) Implemented delivery observability registry in `/Users/rajeshk/code/go/go-toolkit/setu/backend/deliverytracking/metrics.go` with bounded label domains, stage/reason canonicalization, read-code canonicalization, stage-error wrapping, and durable-state status count materialization for `delivery_status_current_count`.
- [x] (2026-02-11 07:45Z) Wired deterministic instrumentation and key=value logs in `/Users/rajeshk/code/go/go-toolkit/setu/backend/deliverytracking/processor.go` for correlation outcomes, ignored/no-op outcomes, committed status/freshness transitions, and freshness evaluator transitions/failures.
- [x] (2026-02-11 07:45Z) Added webhook-ingestion stage attribution wrapping in `/Users/rajeshk/code/go/go-toolkit/setu/backend/deliverytracking/webhook_ingestion.go` to distinguish `webhook_ingestion` vs `correlation_handoff` internal failures.
- [x] (2026-02-11 07:46Z) Added delivery runtime observability wiring in `/Users/rajeshk/code/go/go-toolkit/setu/backend/cmd/delivery-tracking/handlers.go`, `/Users/rajeshk/code/go/go-toolkit/setu/backend/cmd/delivery-tracking/routes.go`, and `/Users/rajeshk/code/go/go-toolkit/setu/backend/cmd/delivery-tracking/main.go`, including `/metrics` route behavior and read API request telemetry.
- [x] (2026-02-11 07:47Z) Added targeted observability tests in `/Users/rajeshk/code/go/go-toolkit/setu/backend/deliverytracking/metrics_test.go` and `/Users/rajeshk/code/go/go-toolkit/setu/backend/cmd/delivery-tracking/main_test.go`.
- [x] (2026-02-11 07:48Z) Ran relevant package tests and captured concrete passing evidence; finalized retrospective.

## Surprises & Discoveries

- Observation: Delivery runtime currently does not expose `/metrics` and has no delivery metric registry.
  Evidence: `/Users/rajeshk/code/go/go-toolkit/setu/backend/cmd/delivery-tracking/routes.go` only registers `/healthz`, `/readyz`, webhook, and read routes.
- Observation: Delivery core and evaluator are implemented in `backend/deliverytracking/processor.go`, but runtime wiring currently instantiates only webhook ingestor and reader.
  Evidence: `/Users/rajeshk/code/go/go-toolkit/setu/backend/cmd/delivery-tracking/main.go` constructs `NewWebhookIngestor` and `NewReader` only.
- Observation: Fixed wall-clock timestamps made evaluator observability assertions brittle because SQL Server clock can differ from hardcoded test dates.
  Evidence: Initial `TestProcessorWithMetricsEmitsFreshnessEvaluatorAndFailureMetrics` expected one stale transition and received `0`; switching to `time.Now().UTC()`-relative timestamps made it deterministic.
- Observation: Forcing read API storage failure by dropping `submission_intents` is blocked by FK constraints in the schema.
  Evidence: test error `Could not drop object 'dbo.submission_intents' because it is referenced by a FOREIGN KEY constraint`; replaced failure trigger with invalid `delivery_tracking_mode` value to force deterministic `500` materialization error.

## Decision Log

- Decision: Keep all delivery observability implementation under delivery-tracking ownership (`backend/deliverytracking` + `backend/cmd/delivery-tracking`) and do not modify submission-manager observability surfaces.
  Rationale: Preserves bounded-context ownership and follows current delivery runtime topology separation.
  Date/Author: 2026-02-11 / Codex
- Decision: Implement bounded-label canonicalization and fallback mapping inside delivery metric registry, with `unknown_reason` fallback where reason mapping can be ambiguous.
  Rationale: Enforces deterministic label domains and prevents cardinality drift from raw error text.
  Date/Author: 2026-02-11 / Codex
- Decision: Attach ingestion internal failure stage information using wrapped stage errors (`WrapProcessingStageError`) and map stage at the HTTP boundary.
  Rationale: Allows deterministic distinction between `webhook_ingestion` and `correlation_handoff` failure metrics while keeping HTTP status mapping unchanged (`503`).
  Date/Author: 2026-02-11 / Codex
- Decision: Keep processor/evaluator instrumentation in `deliverytracking` with optional metrics injection (`NewProcessorWithMetrics` / `newSQLStoreWithMetrics`) and always-on key=value logging.
  Rationale: Preserves core semantics and keeps observability additive while allowing deterministic unit/integration verification without coupling to submission-manager runtime ownership.
  Date/Author: 2026-02-11 / Codex

## Outcomes & Retrospective

Observability slice implementation is complete for delivery-tracking runtime ownership. Delivery runtime now exposes `/metrics` with all required `delivery_` metric families, including canonical `delivery_status_current_count{delivery_status}` materialized from durable state at scrape time. Webhook-ingestion reject/failure paths, read API request/response outcomes, and processor/evaluator outcomes are instrumented with bounded labels and deterministic stage/reason mapping.

Core-processing semantics are preserved: no transaction boundary changes were introduced for delivery state/history mutation behavior, and observability emission remains additive. Existing core-processing and webhook/read tests continue to pass under the updated instrumentation.

No blockers remain for this slice. Dashboarding/alert policy and non-scope security concerns remain deferred by design.

## Context and Orientation

The delivery-tracking runtime lives in `/Users/rajeshk/code/go/go-toolkit/setu/backend/cmd/delivery-tracking`. HTTP handler logic is in `handlers.go`, route wiring is in `routes.go`, and process wiring is in `main.go`. The delivery core package is `/Users/rajeshk/code/go/go-toolkit/setu/backend/deliverytracking`, where webhook ingestion persistence is in `webhook_ingestion.go`, correlation/core apply and freshness evaluation are in `processor.go`, and read materialization is in `reader.go`.

This slice is observability-only. It must not change delivery domain state transition semantics, request payload contracts, or security behavior. Durable queueing and provider polling remain out of scope.

In this plan, "bounded labels" means fixed allowlists for metric label values. "Canonical status/freshness" means the required values from spec: delivery status `{unknown,in_progress,delivered,failed}` and delivery freshness `{fresh,stale,not_applicable}`.

## Plan of Work

Add a delivery metric registry in `/Users/rajeshk/code/go/go-toolkit/setu/backend/deliverytracking` that tracks required counters in-memory and materializes `delivery_status_current_count{delivery_status}` from SQL snapshot reads at scrape time. This registry will expose a `WritePrometheus` method and deterministic label canonicalization helpers.

Integrate the registry into delivery runtime wiring in `/Users/rajeshk/code/go/go-toolkit/setu/backend/cmd/delivery-tracking/main.go` and expose `/metrics` in handlers/routes with the same behavior convention already used elsewhere: `GET` only, `404` when registry is nil, and `text/plain; version=0.0.4` output.

Instrument webhook ingestion HTTP reject/failure paths in `/Users/rajeshk/code/go/go-toolkit/setu/backend/cmd/delivery-tracking/handlers.go`, including required failure metric increments and key=value logs. Instrument read API request accounting by endpoint template and response code with bounded code mapping.

Instrument core processing and freshness evaluator outcome points in `/Users/rajeshk/code/go/go-toolkit/setu/backend/deliverytracking/processor.go` so provider signal classification, ignored/no-op reasons, committed status/freshness transitions, and processing failures are emitted deterministically while preserving transaction-boundary semantics.

Add and adjust tests in `/Users/rajeshk/code/go/go-toolkit/setu/backend/deliverytracking` and `/Users/rajeshk/code/go/go-toolkit/setu/backend/cmd/delivery-tracking` to prove required metric presence, bounded labels, deterministic placement, and non-regression of core semantics.

## Concrete Steps

From `/Users/rajeshk/code/go/go-toolkit/setu`:

1. Add delivery observability metric registry and helper mappings in `backend/deliverytracking`.
2. Update delivery processor/webhook/read handlers to emit required metrics/logs at deterministic boundaries.
3. Add `/metrics` handler wiring in `backend/cmd/delivery-tracking`.
4. Change working directory to `/Users/rajeshk/code/go/go-toolkit/setu/backend`.
5. Run focused tests:
   `go test -count=1 ./deliverytracking -run 'TestDeliveryMetrics|TestProcessorWithMetrics|TestProcessorObservabilityLogsDecisionEvents|TestWebhookIngest|TestDeliveryApply|TestProcessorEvaluateDeliveryFreshnessStaleness'`
6. Run runtime HTTP tests:
   `go test -count=1 ./cmd/delivery-tracking`
7. Run broader relevant package tests:
   `go test -count=1 ./deliverytracking ./cmd/delivery-tracking`

## Validation and Acceptance

Acceptance is met when tests and metrics output prove all required `delivery_` metrics exist with bounded labels, webhook invalid/internal failure paths map to required failure metrics and logs, apply/evaluator transitions emit only from committed outcomes, read API telemetry emits exactly one request counter increment per completed endpoint request, and delivery status current counts are materialized from durable state into canonical status labels.

## Idempotence and Recovery

All changes are additive and safe to rerun. If a test fails mid-run, rerunning the same `go test` commands is safe because test databases are per-test and cleanup is already built into existing test helpers.

## Artifacts and Notes

Test evidence from `/Users/rajeshk/code/go/go-toolkit/setu/backend`:

    $ go test -count=1 ./deliverytracking -run 'TestDeliveryMetrics|TestProcessorWithMetrics|TestProcessorObservabilityLogsDecisionEvents|TestWebhookIngest|TestDeliveryApply|TestProcessorEvaluateDeliveryFreshnessStaleness'
    ok  	gateway/deliverytracking	9.208s

    $ go test -count=1 ./cmd/delivery-tracking
    ok  	gateway/cmd/delivery-tracking	5.469s

    $ go test -count=1 ./deliverytracking ./cmd/delivery-tracking
    ok  	gateway/deliverytracking	12.263s
    ok  	gateway/cmd/delivery-tracking	10.424s

## Interfaces and Dependencies

No new third-party dependencies are planned. The implementation will reuse existing Go standard library packages and current SQL access patterns.

---

Revision note (2026-02-11 / Codex): Initial observability execplan created with `EXECPLAN-READY` and deterministic implementation plan before code changes.
Revision note (2026-02-11 / Codex): Updated plan after implementation with final progress state, decisions, discoveries, and concrete test evidence.
