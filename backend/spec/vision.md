# Setu Vision & Goals

## Vision

Setu is a resilient, organization-wide gateway to external systems. It provides a single, consistent interface that internal applications use to communicate with the outside world.

## Mission

Offer a dependable submission and delivery layer that hides provider complexity, enforces contracts, and delivers clear, auditable outcomes to clients.

## Long-term goals

- Unified external interface: internal applications integrate with external providers through one API.
- Resilient execution over time: retries, timing, and orchestration are owned centrally.
- Trustworthy outcomes: results are normalized, auditable, and faithfully communicated back to clients.
- End-to-end lifecycle: extend beyond submission to delivery confirmations, provider status checks, and delayed responses.
- Operational clarity: contracts and policies are explicit; behavior is deterministic and observable.

## Current scope

- Submission intents, contract-bound execution, and retry orchestration.
- Normalized gateway outcomes and durable runtime state.
- Thin HTTP interface for submit/query.

## Evolving scope

- Delivery tracking and provider status reconciliation.
- Multi-instance coordination and scaling.
- Rate limiting, authn/z, and operator controls.

## Principles

- Clear separation of concerns (gateway semantics vs contract policy vs orchestration).
- Explicit contracts and deterministic behavior.
- Minimal, stable client surface; internal evolution without breaking clients.
