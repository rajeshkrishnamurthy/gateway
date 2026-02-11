# Delivery Tracking V1 Topology Alignment Implementation

This execplan is a living document. The sections `Progress`, `Surprises & Discoveries`, `Decision Log`, and `Outcomes & Retrospective` must be kept up to date as work proceeds.

This document is maintained in accordance with `/Users/rajeshk/code/go/go-toolkit/setu/backend/PLANS.md`.

EXECPLAN-READY

## Purpose / Big Picture

After this change, delivery-tracking route ownership is explicit and enforced at runtime boundaries. `provider signal webhook` and delivery read endpoints are served by a dedicated delivery-tracking runtime behind HAProxy, while SubmissionManager remains submission-only and does not host or proxy delivery-tracking routes. Core processing semantics remain unchanged.

A novice can verify behavior by running focused tests for delivery runtime route ownership and response equivalence, plus SubmissionManager ownership-boundary tests and existing delivery core-processing tests.

## Progress

- [x] (2026-02-11 03:54Z) Read all required EXEC inputs in the exact mandated order.
- [x] (2026-02-11 03:55Z) Reconnaissance completed for current runtime and routing state (`cmd/submission-manager`, HAProxy, docker-compose).
- [x] (2026-02-11 04:18Z) Added dedicated delivery-tracking runtime in `/Users/rajeshk/code/go/go-toolkit/setu/backend/cmd/delivery-tracking` with webhook-ingestion and delivery-read route ownership.
- [x] (2026-02-11 04:20Z) Removed delivery route ownership from SubmissionManager runtime and enforced non-ownership for `/v1/intents/{intentId}/delivery*`.
- [x] (2026-02-11 04:22Z) Added delivery read materializer in `/Users/rajeshk/code/go/go-toolkit/setu/backend/deliverytracking/reader.go` with deterministic transaction-scoped reads and mode-gated shaping.
- [x] (2026-02-11 04:24Z) Updated Docker Compose and HAProxy Docker config for two delivery-tracking instances behind HAProxy frontend `:8083`.
- [x] (2026-02-11 04:27Z) Added route-ownership and cross-instance equivalence tests in delivery runtime and submission-manager runtime test suites.
- [x] (2026-02-11 04:37Z) Ran relevant package tests, captured concrete evidence, and isolated unrelated pre-existing failure.
- [x] (2026-02-11 04:37Z) Finalized outcomes and implementation notes.

## Surprises & Discoveries

- Observation: Delivery webhook-ingestion was wired directly into SubmissionManager runtime in the current workspace state.
  Evidence: `/Users/rajeshk/code/go/go-toolkit/setu/backend/cmd/submission-manager/routes.go` includes `/v1/delivery/provider-signal-webhook`.
- Observation: Delivery read endpoints are not currently implemented in any runtime, so topology alignment must include introducing these routes in delivery-tracking runtime for ownership completeness.
  Evidence: repository search found no existing `/v1/intents/{intentId}/delivery` or `/v1/intents/{intentId}/delivery/history` handlers.
- Observation: SQL Server driver in this repository environment rejects `ReadOnly=true` transaction options.
  Evidence: initial test failure reported `read-only transactions are not supported`; using serializable transactions without `ReadOnly` resolved reader tests.
- Observation: A broader SubmissionManager test remains pre-existing/unrelated to topology changes.
  Evidence: `go test ./cmd/submission-manager` fails at `TestSubmitWaitSecondsEarlyReturn` with elapsed wait near 5 seconds.

## Decision Log

- Decision: Implement a new dedicated runtime at `/Users/rajeshk/code/go/go-toolkit/setu/backend/cmd/delivery-tracking` rather than overloading `cmd/submission-manager`.
  Rationale: Frozen topology spec/design require explicit runtime ownership separation and no SubmissionManager compatibility hosting.
  Date/Author: 2026-02-11 / Codex
- Decision: Use HAProxy frontend `:8083` for delivery-tracking runtime in Docker topology.
  Rationale: Existing stable ports `:8080`, `:8081`, and `:8082` are already allocated; `:8083` keeps routing explicit and additive.
  Date/Author: 2026-02-11 / Codex
- Decision: Keep delivery core-processing package behavior unchanged and add read materialization as read-only queries in `backend/deliverytracking`.
  Rationale: User requested topology/ownership alignment without core-processing semantic drift.
  Date/Author: 2026-02-11 / Codex
- Decision: For delivery read routes in SubmissionManager, return `404 not_found` for `/v1/intents/{intentId}/delivery*` rather than treating them as malformed submission routes.
  Rationale: This enforces explicit non-ownership while preserving existing submission API ownership for `/v1/intents/{intentId}` and `/history`.
  Date/Author: 2026-02-11 / Codex
- Decision: Use serializable transaction boundaries for delivery reads, without `ReadOnly=true`.
  Rationale: Maintains one deterministic read boundary per request while remaining compatible with the SQL Server driver behavior in this environment.
  Date/Author: 2026-02-11 / Codex

## Outcomes & Retrospective

Topology alignment is complete for this EXEC slice. Delivery-tracking route ownership now lives in dedicated runtime `/Users/rajeshk/code/go/go-toolkit/setu/backend/cmd/delivery-tracking` for:

- `POST /v1/delivery/provider-signal-webhook`
- `GET /v1/intents/{intentId}/delivery`
- `GET /v1/intents/{intentId}/delivery/history`

SubmissionManager no longer serves delivery webhook ingress and explicitly returns not-found for delivery read subpaths. HAProxy and Docker Compose now include two delivery-tracking instances behind dedicated frontend `:8083`.

Core processing semantics were preserved: no modifications were made to `processor.go` transition logic, ordering rules, or core apply semantics. New delivery read logic is read-only materialization against persisted state/history and intent snapshot data.

## Context and Orientation

Current delivery core-processing and webhook-ingestion logic exists in `/Users/rajeshk/code/go/go-toolkit/setu/backend/deliverytracking`. SubmissionManager HTTP routing currently lives in `/Users/rajeshk/code/go/go-toolkit/setu/backend/cmd/submission-manager` and presently includes delivery webhook ingress, which violates updated ownership specs.

HAProxy Docker config at `/Users/rajeshk/code/go/go-toolkit/setu/backend/conf/docker/haproxy_docker.cfg` currently fronts SMS (`:8080`), push (`:8081`), and SubmissionManager (`:8082`) only. Docker Compose currently runs two SubmissionManager instances and no dedicated delivery-tracking runtime service.

For this task, “topology alignment” means moving runtime ownership of delivery-tracking routes to a dedicated service while preserving deterministic delivery core semantics and ensuring multi-instance correctness without sticky-session dependence.

## Plan of Work

Implementation followed this sequence.

First, `/Users/rajeshk/code/go/go-toolkit/setu/backend/deliverytracking/reader.go` introduced read materialization methods for current delivery view and delivery history view. Reads are transaction-scoped, mode-aware, and deterministic; tracked intents with sparse state use canonical fallback (`unknown` status with freshness from staleness threshold against persisted intent creation time).

Second, `/Users/rajeshk/code/go/go-toolkit/setu/backend/cmd/delivery-tracking` was added as dedicated runtime ownership boundary with HTTP adapters and wiring for webhook-ingestion and delivery read endpoints.

Third, SubmissionManager runtime ownership was reduced to submission-only behavior by removing delivery webhook route registration and returning not-found for delivery read route subpaths.

Fourth, `/Users/rajeshk/code/go/go-toolkit/setu/backend/conf/docker/haproxy_docker.cfg` and `/Users/rajeshk/code/go/go-toolkit/setu/docker-compose.yml` were updated so two delivery-tracking instances are deployable behind HAProxy frontend `:8083`.

Fifth, test suites were updated to verify route ownership boundaries and cross-instance response equivalence, then relevant package tests were run and captured below.

## Concrete Steps

From `/Users/rajeshk/code/go/go-toolkit/setu`:

1. Added reader implementation and tests under:
   - `/Users/rajeshk/code/go/go-toolkit/setu/backend/deliverytracking/reader.go`
   - `/Users/rajeshk/code/go/go-toolkit/setu/backend/deliverytracking/reader_test.go`
2. Added dedicated runtime under:
   - `/Users/rajeshk/code/go/go-toolkit/setu/backend/cmd/delivery-tracking/`
3. Removed delivery ownership in SubmissionManager adapters/tests:
   - `/Users/rajeshk/code/go/go-toolkit/setu/backend/cmd/submission-manager/handlers.go`
   - `/Users/rajeshk/code/go/go-toolkit/setu/backend/cmd/submission-manager/main_test.go`
4. Updated topology config:
   - `/Users/rajeshk/code/go/go-toolkit/setu/backend/conf/docker/haproxy_docker.cfg`
   - `/Users/rajeshk/code/go/go-toolkit/setu/docker-compose.yml`
5. Ran from `/Users/rajeshk/code/go/go-toolkit/setu/backend`:
   - `go test ./deliverytracking -run 'Test(Reader|Processor|WebhookIngest)'`
   - `go test ./cmd/delivery-tracking`
   - `go test ./cmd/submission-manager -run 'Test(SubmissionManagerDoesNotExposeDelivery|SubmitIntentIdempotent|GetIntent|GetIntentHistory|SubmitUnknownTarget)'`
   - `go test ./deliverytracking ./cmd/delivery-tracking ./cmd/submission-manager`
   - `go test ./cmd/submission-manager -run '^TestSubmitWaitSecondsEarlyReturn$' -count=1`

## Validation and Acceptance

Acceptance is demonstrated by evidence that:

- delivery webhook and delivery read routes exist and return deterministic responses from delivery runtime.
- SubmissionManager does not expose or proxy delivery-tracking routes.
- two delivery runtime instances behind HAProxy can serve equivalent read responses over unchanged persisted state.
- no sticky-session requirement exists for correctness.
- core-processing regression tests continue to pass without semantic changes.

## Idempotence and Recovery

Changes are additive and idempotent at config/runtime level. If delivery runtime startup fails, reverting HAProxy frontend/backend entries and compose services restores prior topology quickly without schema rollback. Core-processing tables and semantics remain unchanged.

## Artifacts and Notes

Test evidence from `/Users/rajeshk/code/go/go-toolkit/setu/backend`:

    $ go test ./deliverytracking -run 'Test(Reader|Processor|WebhookIngest)'
    ok  	gateway/deliverytracking	7.534s

    $ go test ./cmd/delivery-tracking
    ok  	gateway/cmd/delivery-tracking	6.308s

    $ go test ./cmd/submission-manager -run 'Test(SubmissionManagerDoesNotExposeDelivery|SubmitIntentIdempotent|GetIntent|GetIntentHistory|SubmitUnknownTarget)'
    ok  	gateway/cmd/submission-manager	5.766s

    $ go test ./deliverytracking ./cmd/delivery-tracking ./cmd/submission-manager
    ok  	gateway/deliverytracking	10.626s
    ok  	gateway/cmd/delivery-tracking	(cached)
    --- FAIL: TestSubmitWaitSecondsEarlyReturn (5.50s)
        main_test.go:252: expected early return, elapsed 5.009217958s
    FAIL
    FAIL	gateway/cmd/submission-manager	11.356s
    FAIL

    $ go test ./cmd/submission-manager -run '^TestSubmitWaitSecondsEarlyReturn$' -count=1
    --- FAIL: TestSubmitWaitSecondsEarlyReturn (5.45s)
        main_test.go:252: expected early return, elapsed 5.011707083s
    FAIL
    FAIL	gateway/cmd/submission-manager	5.674s
    FAIL

## Interfaces and Dependencies

No new external library dependency is required.

Implemented interfaces:

- In `backend/deliverytracking`, add read-only materialization methods on a concrete reader type:
  - `GetCurrentDelivery(ctx context.Context, intentID string) (CurrentDeliveryView, bool, error)`
  - `GetDeliveryHistory(ctx context.Context, intentID string) (DeliveryHistoryView, bool, error)`
- In `backend/cmd/delivery-tracking`, existing `deliverytracking.WebhookIngestor` and new `deliverytracking.Reader` are wired behind HTTP route adapters for webhook-ingestion and delivery-read endpoints.

---

Revision note (2026-02-11 / Codex): Initial topology-alignment execplan created after mandated read order and runtime boundary reconnaissance.
Revision note (2026-02-11 / Codex): Updated living sections after implementation, recorded runtime/topology decisions, and added concrete test evidence including unrelated pre-existing failure isolation.
