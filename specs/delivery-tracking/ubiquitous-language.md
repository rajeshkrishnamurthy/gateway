# Delivery Tracking: Ubiquitous Language

## Purpose

Define the canonical terms used by delivery-tracking specs so intent stays stable across SPEC, EXEC, and VERIFY work.

## Scope

This glossary applies to the delivery-tracking docs in `specs/delivery-tracking/`.

## First-Class Terms

- `submission status`
  - Meaning: canonical submission lifecycle representation (`accepted`, `rejected`, `exhausted`).
  - Not this: provider delivery input or delivery freshness.

- `delivery status`
  - Meaning: canonical delivery lifecycle representation (`unknown`, `in_progress`, `delivered`, `failed`).
  - Not this: submission status or provider delivery input.

- `terminal delivery status`
  - Meaning: `delivered` or `failed`.
  - Not this: `unknown` or `in_progress`.

- `delivery freshness`
  - Meaning: timeliness dimension for unresolved delivery (`fresh`, `stale`, `not_applicable`).
  - Not this: submission status or delivery status.

- `provider delivery signal`
  - Meaning: raw provider-originated delivery input before correlation and canonical interpretation.
  - Not this: canonical delivery status.

- `delivery history`
  - Meaning: append-only sequence of accepted, correlated provider delivery signals used to explain current delivery status and delivery freshness.
  - Not this: attempt history for submission execution.

- `delivery tracking mode`
  - Meaning: per-target contract switch (`on` or `off`) controlling whether delivery status and delivery freshness are tracked for intents on that target.
  - Not this: runtime feature flag supplied by client request.

- `staleAfterSeconds`
  - Meaning: per-target contract value used for delivery freshness staleness classification when delivery tracking mode is `on`.
  - Not this: delivery timeout, submission timeout, or delivery success/failure rule.

- `correlation result`
  - Meaning: one of `matched`, `unmatched`, or `invalid`.
  - Not this: submission status or delivery status.

- `delivery read API`
  - Meaning: client-to-Setu retrieval contract for delivery status, delivery freshness, and delivery history.
  - Not this: provider signal ingestion.

- `provider signal webhook`
  - Meaning: provider-to-Setu push ingress for provider delivery signals.
  - Not this: client-facing read API.

- `provider signal poll`
  - Meaning: Setu-to-provider pull ingress for provider delivery signals.
  - Not this: canonical delivery status.

## Ambiguity Guardrails

- Unqualified `status` is not allowed in delivery-tracking specs.
- `status` must refer only to canonical correlated representations: `submission status` or `delivery status`.
- Use `delivery freshness` only for timeliness; it is not a status.
- Use `correlation result` for `matched`/`unmatched`/`invalid`; do not call it status.
- Use `provider delivery signal` for raw provider input; do not use signal to mean canonical status.
