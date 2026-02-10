# Delivery Tracking Module: High-Level Intent

## Purpose / Big Picture

Extend Setu beyond submission-status-only behavior by capturing and surfacing delivery confirmations (or failures) from external providers. Delivery tracking is an additional, best-effort layer that complements existing SubmissionManager submission status without redefining it.
Canonical terminology for this module is defined in `specs/delivery-tracking/ubiquitous-language.md`.

## Scope

In scope:

- Contract-level delivery tracking enablement per `submissionTarget`.
- Collect provider delivery signals from external providers (provider signal webhook and/or provider signal poll).
- Normalize those signals into a coherent, provider-agnostic delivery view.
- Reconcile provider delivery signals to existing intents without changing submission status.
- Expose delivery information to clients and operators as a separate dimension.
- Preserve clear coverage boundaries so delivery tracking represents observed delivery context, not inferred delivery truth.

## Non-goals

Out of scope:

- Changing the existing submission contracts or submission status meanings.
- Guaranteeing delivery or providing end-to-end delivery SLAs.
- Real-time delivery guarantees for all providers.
- Provider-specific features that do not map to a common delivery concept.

## Invariants

- Delivery tracking never changes SubmissionManager submission status (`accepted`, `rejected`, `exhausted`).
- Delivery tracking never changes submission completion timestamps.
- Delivery information is best-effort and may be missing or delayed.
- Delivery data is additive: it provides more context but never invalidates submission status.
- If no authoritative provider delivery signal exists, delivery status remains `unknown`; delivery tracking must not infer a terminal delivery result.
- Normalization must preserve meaning and avoid leaking provider-specific semantics to clients.
- Delivery tracking is controlled only by contract config on `submissionTarget`: `deliveryTracking.mode` (`on` or `off`, default `off`).
- When `deliveryTracking.mode` is `on`, `deliveryTracking.staleAfterSeconds` is configured per target and controls freshness staleness classification.
- Staleness for unresolved delivery is a timeliness classification only and must not be interpreted as terminal delivery failure.
- Correlation fallback handling, ambiguity handling, terminal conflict handling, and stale-to-fresh behavior are fixed internal semantics, not public per-target config.

## Workflows (High-Level)

- Contract gating: resolve delivery tracking settings from the intent's `submissionTarget` contract snapshot (`mode`, `staleAfterSeconds`).
- Provider signal ingestion: accept provider delivery signals from providers that support push ingress and, when in scope, pull ingress.
- Provider signal reconciliation: map each provider delivery signal to the corresponding intent and update delivery status and delivery history.
- Freshness classification: classify unresolved delivery status as stale when expected observation conditions are no longer met.
- Client visibility: surface delivery status, delivery freshness, and delivery history alongside, but separate from, submission status.
- Operator visibility: show delivery status, delivery freshness, delays, and gaps for operational diagnosis.

## Variations We Must Address

- Providers that push provider delivery signals vs providers that only support provider signal poll.
- Providers that emit multiple delivery status values vs a single final delivery status.
- Duplicate or out-of-order provider delivery signals.
- Late provider delivery signals that arrive after the submission status is terminal.
- Missing provider delivery signals that never arrive.
- Different `submissionTarget` contracts can have different `staleAfterSeconds` values.

## Race Conditions and Handling

- Provider delivery signals may arrive before a client observes acceptance; the system must converge to a stable delivery view regardless of observation order.
- Duplicate provider delivery signals must be safe and must not create conflicting delivery statuses.
- Out-of-order provider delivery signals must not produce inconsistent client-visible delivery history.

## Failure Semantics

- If provider signal ingestion fails, submission status remains authoritative and unchanged.
- If a provider does not supply provider delivery signals, delivery status remains `unknown` rather than inferred.
- Incomplete delivery data must be explicitly represented as unknown or pending.
- Staleness classification does not imply delivery failure; it only indicates unresolved delivery beyond expected observation conditions.
- If delivery tracking mode is `off` for an intent's contract snapshot, delivery tracking workflows are not applied for that intent.

## Concurrency Guarantees

- Provider delivery signal processing is order-independent: the final delivery view must be consistent regardless of provider delivery signal arrival order.
- Concurrent provider delivery signals for the same intent must not produce divergent delivery statuses.

## Observable Acceptance Criteria

- Clients and operators can clearly distinguish submission status from delivery status.
- Clients can query delivery status separately from submission status for a given intent.
- Operators can identify intents with missing or delayed provider delivery signals.
- Delivery status is explicitly `unknown` when no authoritative provider delivery signal exists.
- Clients and operators can identify unresolved intents where delivery freshness has transitioned to `stale`.
- Delivery data is visibly marked as best-effort and not authoritative for submission status.
- If a delivery success signal arrives after submission is already terminal (for example, `exhausted`), submission status and completion timestamps remain unchanged while delivery records a late success observation.
- Contracts with delivery tracking `off` do not produce delivery status or delivery freshness changes.
- Contracts with delivery tracking `on` classify staleness using that target's `staleAfterSeconds`.

## Requires DESIGN decision

- Requires DESIGN decision: deterministic definition of "newer valid signal" used by freshness reclassification.
