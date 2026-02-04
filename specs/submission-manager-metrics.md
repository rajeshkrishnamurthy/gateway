# Submission Manager metrics
COMPLETED

## Purpose

This document specifies the Prometheus metrics that SubmissionManager emits. The metric set is intentionally separate from gateway metrics to avoid overlap and to reflect orchestration outcomes only.

## Principles

- Low-cardinality labels only (status, policy, gatewayType).
- No intentId, submissionTarget, payload-derived labels.
- Counters and histograms are preferred over gauges unless state is naturally instantaneous.
- Metrics must reflect SubmissionManager decisions and timing, not gateway/provider internals.
- Metrics must not duplicate gateway metrics (no provider labels, no gateway request totals).

## Counters

- `submission_intents_created_total`
  - Total intents created (new intentId).

- `submission_idempotent_hits_total`
  - Idempotent re-submissions with the same intentId + submissionTarget + payload.

- `submission_idempotency_conflicts_total`
  - Idempotency conflicts (same intentId, different submissionTarget or payload).

- `submission_intents_terminal_total{status}`
  - Terminal intents by final status.
  - `status` is one of: `accepted`, `rejected`, `exhausted`.

- `submission_exhausted_total{reason}`
  - Exhausted intents by reason.
  - `reason` is one of: `deadline_exceeded`, `max_attempts`, `one_shot`, `unknown_policy`, `unknown_reason`.

- `submission_attempts_total{outcome_status}`
  - Attempt outcomes by status.
  - `outcome_status` is one of: `accepted`, `rejected`, `error`.

- `submission_retries_scheduled_total`
  - Count of retries scheduled (non-terminal attempts that result in a new due time).

## Histograms

- `submission_intent_time_to_terminal_seconds{status}`
  - Time from intent creation to terminal completion.
  - `status` is one of: `accepted`, `rejected`, `exhausted`.

- `submission_attempt_duration_seconds`
  - Attempt execution duration (from attempt start to finish).

- `submission_queue_delay_seconds`
  - Scheduling lag: `now - next_attempt_at` at time of execution.

## Gauges

- `submission_queue_depth`
  - Count of intents waiting for execution (scheduled attempts).

- `submission_inflight_attempts`
  - Count of attempts currently executing.

## Optional labels (only if needed)

If needed for operational slicing, the following labels may be added with caution:

- `gatewayType` (sms|push)
- `policy` (deadline|max_attempts|one_shot)

Do not add labels for submissionTarget, intentId, or payload fields.

## Grafana dashboards

The Grafana dashboard for SubmissionManager metrics is:

- `backend/conf/grafana/dashboards/submission-manager-overview.json`
