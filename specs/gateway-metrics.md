# Gateway metrics
COMPLETED

## Purpose

This document defines the Prometheus metrics emitted by SMS and push gateways. Metrics are low-cardinality and use the provider name as a label.

## Provider label

All metrics include a `provider` label that is set to the configured adapter provider name.

## Counters

- `gateway_requests_total{provider}`
  - Total gateway requests.

- `gateway_outcomes_total{provider,outcome}`
  - Outcome counts by `accepted` and `rejected`.

- `gateway_rejections_total{provider,reason}`
  - Rejections by reason. Reasons include:
    - `invalid_request`
    - `duplicate_reference`
    - `invalid_recipient`
    - `invalid_message`
    - `provider_failure`
    - `unregistered_token`

- `gateway_provider_failures_total{provider}`
  - Count of provider failures (error return or normalized failures).

- `gateway_provider_timeouts_total{provider}`
  - Count of provider call timeouts (context deadline exceeded).

- `gateway_provider_panics_total{provider}`
  - Count of provider panics recovered by the gateway.

## Histograms

- `gateway_request_duration_seconds{provider,le}`
  - End-to-end request duration in seconds.

- `gateway_provider_duration_seconds{provider,le}`
  - Provider call duration in seconds.

Buckets are fixed for both histograms:

- 0.1s, 0.25s, 0.5s, 1s, 2.5s, 5s

## Grafana dashboards

Gateway overview dashboards live in `backend/conf/grafana/dashboards` and visualize only gateway metrics.
