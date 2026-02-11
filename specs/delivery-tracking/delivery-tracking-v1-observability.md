# Delivery Tracking V1 Slice: Observability

EXEC-READY

## Purpose / Big Picture

This slice defines operator-facing observability for delivery tracking so runtime behavior is diagnosable without inspecting internal code paths. The purpose is to make ingestion, correlation, delivery-status progression, delivery-freshness progression, and mode-off gating visible through stable metrics, logs, Prometheus scraping, and Grafana dashboards.

## Scope

This slice covers:

- metrics emitted for delivery tracking
- structured logs for key delivery-tracking decisions
- audit visibility for ignored signals (`mode_off`)
- read-API access telemetry for delivery endpoints
- Prometheus scraping of delivery-tracking metrics
- Grafana dashboard surfacing for delivery-tracking observability
- admin-portal dashboard linkage for delivery observability

## Non-goals

This slice does not define alert policies, SLO thresholds, provider-specific analytics, or storage of external log systems.

This slice does not change domain behavior for submission status, delivery status, or delivery freshness.

## Normative Inheritance

This slice inherits umbrella and foundational contracts from `specs/delivery-tracking/delivery-tracking-v1.md`, `specs/delivery-tracking/overview.md`, `specs/delivery-tracking/delivery-status-model.md`, `specs/delivery-tracking/delivery-freshness.md`, `specs/delivery-tracking/intent-correlation.md`, and `specs/delivery-tracking/ubiquitous-language.md`.

Portal linkage behavior for dashboards inherits from `specs/admin-portal.md`.

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

## Prometheus Scrape Contract

Prometheus must scrape delivery-tracking runtime instances directly and must not scrape delivery-tracking metrics through HAProxy.

For Docker Compose, `backend/conf/docker/prometheus_docker.yml` must define a scrape job named `delivery-tracking` with `metrics_path: "/metrics"` and targets `delivery-tracking-1:8083` and `delivery-tracking-2:8083`.

For the retained non-Docker path, `backend/conf/prometheus.yml` must define a matching `delivery-tracking` scrape job with `metrics_path: "/metrics"` and host targets for delivery-tracking instances.

Prometheus scrape failures for one or more delivery-tracking targets must degrade only observability visibility and must not alter delivery-tracking domain outcomes.

## Grafana Dashboard Contract

Grafana must provision two delivery-tracking dashboards under `backend/conf/grafana/dashboards`:

- `Delivery Tracking Health` with UID/slug `delivery-tracking-overview`
- `Delivery Tracking Activity` with UID/slug `delivery-tracking-activity`

Both dashboards must query the existing Prometheus datasource (`uid: prometheus`) and must use only low-cardinality label dimensions already defined by this slice.

### Delivery Tracking Health (Trouble Detection)

This dashboard must contain exactly five top-level KPI panels:

1. Processing failure rate by `stage` and `reason` (`delivery_processing_failures_total` over a short rate window).
2. Correlation reject ratio (`correlation_result` in `unmatched|invalid` divided by total provider signals).
3. `stale` transition rate (`delivery_freshness_transitions_total{to_delivery_freshness="stale"}`).
4. Current unresolved load from canonical delivery status (`unknown` and `in_progress`) using `delivery_status_current_count`.
5. Delivery read API error ratio from `delivery_read_api_requests_total` (`500|503|other` divided by total).

For panel 4, gauge aggregation across instances must use `max` (or equivalent non-summing aggregation) and must not use `sum`.

### Delivery Tracking Activity (Past X Minutes/Hours)

This dashboard must be range-aware and must use the selected Grafana time range for rollups (for example via `$__range`).

This dashboard must contain exactly five top-level KPI panels:

1. Provider signal volume in the selected window, broken down by `correlation_result`.
2. Delivery-status transitions in the selected window, grouped by `to_delivery_status`.
3. Delivery-freshness transitions in the selected window, grouped by `to_delivery_freshness`.
4. Ignored signals in the selected window, grouped by `reason`.
5. Delivery read API activity in the selected window, grouped by `endpoint` and `code`, with an explicit error-share view.

## Admin Portal Dashboard Linkage

The admin portal dashboard integration must expose both delivery-tracking dashboards through configured links:

- Health dashboard link to UID/slug `delivery-tracking-overview`
- Activity dashboard link to UID/slug `delivery-tracking-activity`

Portal route behavior, config field names, and dashboard-page rendering rules are governed by `specs/admin-portal.md` and must remain consistent with this slice.

## Invariants

Slice-specific observability invariants are:

- additive emission and scrape behavior (no domain semantics change)
- unchanged domain outcomes when observability backends are missing
- queryable `mode_off` audit visibility through both metrics and logs
- fixed two-dashboard contract (`Delivery Tracking Health` and `Delivery Tracking Activity`) with the mandated five KPI panels each
- deterministic cross-instance handling for `delivery_status_current_count` by using non-summing aggregation

## Race Conditions and Handling

Slice-specific race handling is observability attribution under interleaving: logs and counters remain structurally valid and attributable, and duplicate provider delivery signals do not imply contradictory transition counts.

Concurrent updates and Prometheus scrapes may observe adjacent snapshots; dashboards must represent this as normal time-series behavior and must not infer domain inconsistency solely from near-scrape jitter.

## Failure Semantics

Slice-specific failure behavior is observability degradation without domain failure: emission failures and scrape/query failures do not become domain-processing failures, and delivery processing continues with remaining available channels.

If one delivery-tracking instance is unavailable, Prometheus and Grafana must continue to surface partial telemetry from healthy instances.

## Concurrency Guarantees

Slice-specific guarantees are monotonic, concurrent-safe metric accounting and stable attribution of delivery read API telemetry by endpoint and response code.

Dashboard queries must be read-only and concurrency-safe under multi-instance ingestion and read traffic.

## Observable Acceptance Criteria

Criterion 1: all required `delivery_` metrics are present on the delivery-tracking metrics endpoint.

Criterion 2: `backend/conf/docker/prometheus_docker.yml` includes a `delivery-tracking` scrape job that targets `delivery-tracking-1:8083` and `delivery-tracking-2:8083` with `metrics_path: "/metrics"`.

Criterion 3: `delivery_ignored_signals_total` records `mode_off` events when mode-off signals are encountered.

Criterion 4: logs include structured records for invalid ingestion, non-matched correlation results, and delivery-status/delivery-freshness transitions.

Criterion 5: delivery read API request telemetry exists for both `GET /v1/intents/{intentId}/delivery` and `GET /v1/intents/{intentId}/delivery/history`.

Criterion 6: status-count summaries are available through `delivery_status_current_count{delivery_status}` for canonical values `unknown`, `in_progress`, `delivered`, and `failed`, and dashboard aggregation for this gauge is non-summing.

Criterion 7: Grafana provisions a dashboard titled `Delivery Tracking Health` with UID `delivery-tracking-overview` and exactly the five mandatory trouble-detection KPI panels defined by this slice.

Criterion 8: Grafana provisions a dashboard titled `Delivery Tracking Activity` with UID `delivery-tracking-activity` and exactly the five mandatory range-window KPI panels defined by this slice.

Criterion 9: `Delivery Tracking Activity` panels respond to time-range selection and represent selected-window totals/rates, not fixed-window constants.

Criterion 10: disabling or failing observability outputs does not change submission status, delivery status, or delivery freshness behavior.
