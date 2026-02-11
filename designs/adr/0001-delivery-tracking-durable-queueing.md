# ADR 0001: Delivery Tracking Durable Queueing

## Status
Accepted

## Date
2026-02-11

## Context
Delivery-tracking V1 core processing currently applies matched signals directly to SQL state/history within a transaction. A design question was raised: should V1 add a durable inbox queue between ingress and core apply.

Key reliability clarification:
- Durable queueing does not improve the case where SQL is unavailable at ingress time.
- Durable queueing does improve recovery when a signal is already durably accepted but downstream processing fails, crashes, or is retried.

## Decision
Do **not** include durable queueing in delivery-tracking V1. Defer it to a future phase.

V1 remains:
- best-effort delivery-tracking semantics,
- transactional core apply (history + current projection),
- idempotent apply contract (`intentId + sourceRecordId`) retained so queueing can be added later without semantic change.

## Rationale
- Adding a durable inbox queue in V1 expands scope significantly (ingress persistence model, claim/lock protocol, retry/backoff policy, dead-letter policy, and queue observability).
- This exceeds the bounded V1 objective for deterministic core-processing conventions.
- Deferring preserves momentum while keeping the design queue-ready.

## Consequences
- V1 may require provider resend or manual replay for some post-ingress failure scenarios.
- Duplicate/no-op handling remains mandatory and continues to protect deterministic convergence.
- Future queueing can be introduced as an additive reliability phase with limited semantic churn.

## Follow-up Triggers
Re-open this ADR when either condition becomes required:
- contract expectation shifts from best-effort to eventual-application after acceptance, or
- incident data shows unacceptable loss/replay burden from post-ingress failures.
