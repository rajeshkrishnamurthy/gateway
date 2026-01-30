# Setu Vision and Goals

## Vision

Setu is a resilient, organization-wide gateway to external systems. It provides a
single, consistent interface that internal applications use to communicate with
the outside world.

## Mission

Offer a dependable submission and delivery layer that hides provider complexity,
enforces contracts, and delivers clear, auditable outcomes to clients.

## Long-term goals

- Unified external interface for all integrations.
- Resilient execution over time (retries, delays, transient failures).
- Trustworthy outcomes that are normalized and auditable.
- End-to-end lifecycle support, including delivery confirmations and delayed
  provider status checks.
- Operational clarity with explicit contracts and deterministic behavior.

## Current scope

- Submission intents, contract-bound execution, and retry orchestration.
- Normalized gateway outcomes with durable state.
- Thin HTTP interface for submit/query.

## Principles

- Separation of concerns: gateway semantics vs contract policy vs orchestration.
- Explicit contracts and deterministic behavior.
- Minimal, stable client surface with internal evolution over time.
