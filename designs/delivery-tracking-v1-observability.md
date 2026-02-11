# Design: Delivery Tracking V1 Observability (Deterministic Conventions)

## Scope Guard
This design covers only the observability slice for delivery tracking V1: required metrics, structured logs, mode-off audit visibility, and delivery read API telemetry conventions. It does not design dashboard/alert policy, webhook auth/signature/replay protection, provider signal poll behavior, delivery core semantics, or implementation code.

## Inputs
- `specs/delivery-tracking/delivery-tracking-v1-observability.md`
- `specs/delivery-tracking/delivery-tracking-v1.md`
- `specs/delivery-tracking/delivery-tracking-v1-service-topology.md`
- `specs/delivery-tracking/delivery-tracking-v1-webhook-ingestion.md`
- `specs/delivery-tracking/delivery-tracking-v1-core-processing.md`
- `specs/delivery-tracking/delivery-tracking-v1-get-retrieval.md`
- Foundational delivery-tracking specs:
  - `specs/delivery-tracking/overview.md`
  - `specs/delivery-tracking/delivery-status-model.md`
  - `specs/delivery-tracking/delivery-freshness.md`
  - `specs/delivery-tracking/intent-correlation.md`
  - `specs/delivery-tracking/ubiquitous-language.md`
- Existing V1 design notes:
  - `designs/delivery-tracking-v1-service-topology.md`
  - `designs/delivery-tracking-v1-webhook-ingestion.md`
  - `designs/delivery-tracking-v1-core-processing.md`
  - `designs/delivery-tracking-v1-delivery-read-api.md`

## 1. Decision Statement
Choose one deterministic observability convention set so delivery-tracking runtime behavior is attributable and queryable under duplicate/out-of-order signals and multi-instance routing, without changing domain outcomes.

## 2. Options (1-3)

### Option A: Ad hoc inline metric/log emission at each call site
Short description:
- Each handler/processor path writes counters and logs directly with local string literals.

Pros:
- Lowest up-front design overhead.
- Fastest to start implementing.

Cons:
- High drift risk in metric labels, reason values, and endpoint naming.
- Hard to enforce bounded cardinality and stable semantics.

Risks / failure modes:
- Inconsistent label values across runtime paths.
- Duplicate or missing emission under retries/refactors.

Operational impact:
- Short-term simplicity, long-term maintenance/debug cost.

Compatibility with existing repo patterns:
- Medium. Works technically, but weaker determinism than existing explicit contract style.

### Option B: Outcome-driven observability contract with bounded label/value sets (Recommended)
Short description:
- Emit observability from canonical outcome boundaries (ingestion reject/accept, correlation result, state transition, ignored signal, processing failure, read API response).
- Materialize delivery-status count summaries as a bounded `delivery status` gauge from durable state snapshots.
- Enforce fixed metric names and fixed bounded label/value sets.
- Keep emission best-effort and non-blocking to domain outcomes.

Pros:
- Deterministic, reviewable mapping from runtime outcomes to telemetry.
- Stable metric/log schema under refactors and multi-instance operation.
- Bounded cardinality by construction.

Cons:
- Requires explicit mapping tables for stage/reason and endpoint/code.
- Slightly more implementation plumbing than Option A.

Risks / failure modes:
- Undercount possible if process crashes after commit and before emission.
- Mis-mapping bug can misclassify failure reason until corrected.

Operational impact:
- Predictable troubleshooting and easier dashboard/alert layering later.
- Clear VERIFY assertions for slice acceptance criteria.

Compatibility with existing repo patterns:
- High. Aligns with contract-first deterministic behavior already used in V1 designs.

### Option C: Async buffered observability pipeline
Short description:
- Domain paths enqueue observability events; separate worker emits metrics/logs asynchronously.

Pros:
- Lower synchronous overhead in hot paths.
- Isolates emission backend latency from request latency.

Cons:
- Adds queue lifecycle, backpressure, and loss semantics complexity.
- Introduces new moving parts outside V1 need.

Risks / failure modes:
- Buffer overflow or worker outage can silently drop attribution.
- More difficult failure diagnosis than synchronous best-effort emission.

Operational impact:
- Extra runtime component and monitoring burden.

Compatibility with existing repo patterns:
- Medium. Feasible, but larger than V1 scope.

## 3. Recommendation (Exactly One)
Recommend **Option B**.

Rationale and tradeoffs:
- It gives deterministic observability semantics with bounded cardinality while preserving the spec requirement that observability failures do not alter domain behavior.
- It avoids architecture expansion (Option C) and naming drift (Option A).
- Tradeoff accepted: best-effort post-commit emission can undercount in crash windows.

## 4. Approval Prompt (Exact Text)
Approve recommendation (Option B) for delivery-tracking V1 observability conventions.

## 5. Decision Record (Final Text)
Chosen option:
- Option B: outcome-driven observability contract with bounded label/value sets and best-effort non-blocking emission.

### 5.0 Ownership and topology boundary
- `delivery_` prefixed metrics and delivery-tracking structured logs are emitted by delivery-tracking runtime instances.
- SubmissionManager does not own or dual-serve delivery-tracking route telemetry.
- Correctness of telemetry attribution must not depend on sticky routing; any healthy delivery-tracking instance may emit for a handled request/signal.
- Metrics are exposed on the existing runtime metrics surface (`GET /metrics`) with Prometheus text exposition (`Content-Type: text/plain; version=0.0.4`) consistent with SubmissionManager and gateways.
- Metrics endpoint behavior aligns with existing services: `GET` only, and `404` when metrics registry is not configured.

### 5.1 Canonical metric contract and bounded labels
Required metrics, types, and label domains:

1) `delivery_provider_signals_total{source,correlation_result}`
- Metric type: counter.
- `source` values in V1: `provider signal webhook`.
- `correlation_result` values: `matched`, `unmatched`, `invalid`.
- Counting rule: increment once per correlated signal classification event.

2) `delivery_status_current_count{delivery_status}`
- Metric type: gauge.
- `delivery_status` values: `unknown`, `in_progress`, `delivered`, `failed`.
- Materialization rule:
  - values come from durable-state snapshot query (not monotonic counter increments);
  - include only intents where `delivery tracking mode=on`;
  - `unknown` includes tracked intents with no current-state row yet, plus rows with current `delivery status=unknown`.
- Aggregation note for multi-instance runtime:
  - each instance exposes the same snapshot semantics for the shared durable state;
  - dashboards/alerts should use a non-summing aggregator (`max` or per-instance selection), not `sum`, for this gauge.

3) `delivery_status_transitions_total{from_delivery_status,to_delivery_status}`
- Metric type: counter.
- `from_delivery_status` and `to_delivery_status` values: `unknown`, `in_progress`, `delivered`, `failed`.
- Counting rule: increment only when committed current `delivery status` actually changes (`from != to`).

4) `delivery_freshness_transitions_total{from_delivery_freshness,to_delivery_freshness}`
- Metric type: counter.
- `from_delivery_freshness` and `to_delivery_freshness` values: `fresh`, `stale`, `not_applicable`.
- Counting rule: increment only when committed current `delivery freshness` actually changes.
- Evaluator updates (`fresh -> stale`) are included; signal-apply updates (`stale -> fresh`, `* -> not_applicable`) are included when committed.

5) `delivery_ignored_signals_total{reason}`
- Metric type: counter.
- `reason` values in V1: `mode_off`, `correlation_unmatched`, `correlation_invalid`, `duplicate_source_record`.
- `mode_off` is mandatory for audit visibility.
- Counting rule: increment when processing result is an ignored/no-op class for a signal.

6) `delivery_processing_failures_total{stage,reason}`
- Metric type: counter.
- `stage` values in V1: `webhook_ingestion`, `correlation_handoff`, `core_processing`, `freshness_evaluator`, `delivery_read_api`.
- `reason` values in V1: `invalid_payload`, `storage_unavailable`, `timeout`, `deadlock`, `dependency_unavailable`, `internal_error`, `unknown_reason`.
- Counting rule: increment once per failed operation that prevents expected slice behavior for that stage.

7) `delivery_read_api_requests_total{endpoint,code}`
- Metric type: counter.
- `endpoint` values: `/v1/intents/{intentId}/delivery`, `/v1/intents/{intentId}/delivery/history`.
- `code` values in V1 allowlist: `200`, `400`, `404`, `405`, `500`, `503`; any other response code maps to `other`.
- Counting rule: increment exactly once per completed read API request.

### 5.2 Emission placement conventions
1) Webhook ingestion boundary
- Invalid payload rejection (`400` class) emits:
  - `delivery_processing_failures_total{stage="webhook_ingestion",reason="invalid_payload"}`
  - structured rejection log event.
- Internal ingest failure (`503` class) emits:
  - `delivery_processing_failures_total{stage="webhook_ingestion",reason=<mapped>}`
  - structured failure log event.

2) Correlation/core processing boundary
- Correlation classification emits `delivery_provider_signals_total{source,correlation_result}`.
- Ignored/no-op results emit `delivery_ignored_signals_total{reason=...}` and structured ignored-signal log.
- Committed current-state transitions emit status/freshness transition counters and structured transition logs.
- Opposite-terminal conflict that is history-only does not emit a status-transition counter.

3) Delivery status count gauge boundary
- `delivery_status_current_count{delivery_status}` values are produced from durable-state snapshot reads on metrics scrape.
- Gauge production is independent from request handlers and does not mutate domain state.

4) Freshness evaluator boundary
- Each committed `fresh -> stale` state change emits one `delivery_freshness_transitions_total{from_delivery_freshness="fresh",to_delivery_freshness="stale"}` count.
- Evaluator failure emits `delivery_processing_failures_total{stage="freshness_evaluator",reason=<mapped>}` plus structured failure log.

5) Delivery read API boundary
- Every completed request emits exactly one `delivery_read_api_requests_total{endpoint,code}`.
- Read materialization failure (`500`) also emits `delivery_processing_failures_total{stage="delivery_read_api",reason="internal_error"}`.

### 5.3 Structured log contract
- Consistency convention with SubmissionManager and gateways:
  - use single-line key-value logs from the standard logger (no multiline prose);
  - field keys are stable and lowercase (for example: `event=... intentId=... reason=...`).
  - emit one decision/event log per processed boundary outcome (validation reject, correlation no-op, transition, failure), matching current gateway/submission-manager log style.
- Required log events:
  - ingestion rejection
  - `correlation result` `unmatched`
  - `correlation result` `invalid`
  - `mode_off` ignored signal
  - `delivery status` transition
  - `delivery freshness` transition
  - processing failure preventing expected apply/read behavior
- Required fields when available:
  - `event`
  - `source`
  - `intentId`
  - `submissionTarget`
  - `deliveryTrackingMode`
  - `correlationResult`
  - `reason`
- Safety constraints:
  - No payload/body logging.
  - No provider-secret/header-value logging.
  - Keep field values bounded and machine-parseable.

### 5.4 Failure isolation and transaction alignment
- Observability emission is additive and best-effort; emission failure never changes domain write/read outcomes.
- Transition counters/logs are emitted only from committed outcomes, not speculative pre-commit states.
- If emission backend is unavailable, domain flow continues and only telemetry degrades.

### 5.5 Concurrency and ordering guarantees for telemetry
- Multi-instance delivery runtime may emit concurrently; aggregation is by metric name+label set.
- Duplicate/out-of-order signals must not create contradictory transition counters because counters are tied to committed state-change outcomes.
- Read API telemetry is request-scoped and independent of instance stickiness.

Configuration parameters:
- No new public API or per-target configuration.
- Internal mapping tables for `stage`, `reason`, endpoint templates, and allowed response-code labels are fixed by this design.
- Internal query used to materialize `delivery_status_current_count` is fixed by this design and must preserve canonical `delivery status` semantics.

Constraints that must hold:
- Label cardinality must remain bounded by the allowlists above.
- Route-template labels must be used instead of raw URL path values.
- No metric label may include `intentId`, `submissionTarget`, payload content, or provider-secret-derived values.
- Metric types follow existing repository conventions: use counters for monotonic outcomes and gauges only for instantaneous state summaries.
- Domain semantics (`submission status`, `delivery status`, `delivery freshness`) must not change due to observability emission behavior.

## Risks and Failure Modes (Chosen Option)
- Process crash after commit but before emission can undercount transitions/signals.
- Incorrect stage/reason mapping can skew operator diagnosis until corrected.
- High unmatched/invalid traffic can increase log volume; operational controls (sampling/rate policies) may be needed later.

## Explicit Deferrals
- Dashboard layout and alert policy design are deferred.
- SLO/SLI threshold definitions are deferred.
- Distributed tracing/span schema for delivery-tracking flows is deferred.
- Provider-specific analytics dimensions are deferred.
- Webhook security telemetry for auth/signature/replay protection is deferred to the security-hardening phase.

## ADR (Concise)
- ADR-OBS-001: Use outcome-driven observability emission with bounded label domains.
- ADR-OBS-002: Emit transition counters only from committed state changes.
- ADR-OBS-003: Materialize `delivery_status_current_count` as a durable-state gauge with canonical `delivery status` values.
- ADR-OBS-004: Keep observability best-effort and non-blocking to domain outcomes.
