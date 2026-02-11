# Delivery Tracking V1 Slice: Observability

## Purpose / Big Picture

This slice defines operator-facing observability for delivery tracking so runtime behavior is diagnosable without inspecting internal code paths. The purpose is to make ingestion, correlation, delivery-status progression, delivery-freshness progression, and mode-off gating visible through stable metrics and logs.

## Scope

This slice covers metrics emitted for delivery tracking, structured logs for key delivery-tracking decisions, audit visibility for ignored signals (`mode_off`), and read-API access telemetry for delivery endpoints.

## Non-goals

This slice does not define dashboard layouts, alert policies, provider-specific analytics, or storage of external log systems.

This slice does not change domain behavior for submission status, delivery status, or delivery freshness.

## Normative Inheritance

This slice inherits umbrella and foundational contracts from `specs/delivery-tracking/delivery-tracking-v1.md`, `specs/delivery-tracking/overview.md`, `specs/delivery-tracking/delivery-status-model.md`, `specs/delivery-tracking/delivery-freshness.md`, `specs/delivery-tracking/intent-correlation.md`, and `specs/delivery-tracking/ubiquitous-language.md`.

Unless this slice explicitly adds constraints, invariants, race handling, failure semantics, and concurrency guarantees are governed by those inherited documents.

## Metrics Contract

Delivery-tracking metrics must be exposed through the existing metrics surface and must use the `delivery_` prefix. V1 requires the following metrics:

```text
delivery_provider_signals_total{source,correlation_result}
delivery_status_current_count{delivery_status}
delivery_status_transitions_total{from_delivery_status,to_delivery_status}
delivery_freshness_transitions_total{from_delivery_freshness,to_delivery_freshness}
delivery_ignored_signals_total{reason}
delivery_processing_failures_total{stage,reason}
delivery_read_api_requests_total{endpoint,code}
```

For `delivery_ignored_signals_total`, `reason` must include `mode_off` when applicable.

For `delivery_status_current_count`, `delivery_status` must be the canonical delivery status values: `unknown`, `in_progress`, `delivered`, and `failed`.

## Structured Log Contract

Structured logs must be emitted for provider signal ingestion rejection, correlation results `unmatched` and `invalid`, mode-off ignored-signal handling, delivery-status transitions, delivery-freshness transitions, and processing failures that prevent signal application. Where available, logs must include `intentId`, `submissionTarget`, `deliveryTrackingMode`, `correlationResult`, and `reason`.

## Invariants

Slice-specific observability-boundary invariants are additive emission, unchanged domain outcomes when observability backends are missing, and queryable mode-off audit visibility through both metrics and logs.

## Race Conditions and Handling

Slice-specific race handling is observability attribution under interleaving: logs and counters remain structurally valid and attributable, and duplicate provider delivery signals do not imply contradictory transition counts.

## Failure Semantics

Slice-specific failure behavior is observability degradation without domain failure: emission failures do not become domain-processing failures and processing continues with remaining available observability channels.

## Concurrency Guarantees

Slice-specific guarantee is monotonic, concurrent-safe metric accounting and stable attribution of delivery read API telemetry by endpoint and response code.

## Observable Acceptance Criteria

Criterion 1: all required `delivery_` metrics are present on the metrics endpoint.

Criterion 2: `delivery_ignored_signals_total` records `mode_off` events when mode-off signals are encountered.

Criterion 3: logs include structured records for invalid ingestion, non-matched correlation results, and delivery/freshness transitions.

Criterion 4: delivery read API request telemetry exists for both `GET /v1/intents/{intentId}/delivery` and `GET /v1/intents/{intentId}/delivery/history`.

Criterion 5: disabling or failing observability outputs does not change submission status, delivery status, or delivery freshness behavior.

Criterion 6: status-count summaries are available through `delivery_status_current_count{delivery_status}` for canonical values `unknown`, `in_progress`, `delivered`, and `failed`.
