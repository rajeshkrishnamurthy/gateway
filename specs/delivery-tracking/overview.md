# Delivery Tracking Module: High-Level Intent

## Purpose / Big Picture

Extend Setu beyond submission-only outcomes by capturing and surfacing delivery confirmations (or failures) from external providers. Delivery tracking is an additional, best-effort layer that complements existing SubmissionManager outcomes without redefining them.

## Scope

In scope:

- Collect delivery signals from external providers (callbacks and/or polling).
- Normalize those signals into a coherent, provider-agnostic delivery view.
- Reconcile delivery signals to existing intents without changing submission outcomes.
- Expose delivery information to clients and operators as a separate dimension.

## Non-goals

Out of scope:

- Changing the existing submission contracts or status meanings.
- Guaranteeing delivery or providing end-to-end delivery SLAs.
- Real-time delivery guarantees for all providers.
- Provider-specific features that do not map to a common delivery concept.

## Invariants

- Delivery tracking never changes a SubmissionManager terminal status (accepted/rejected/exhausted).
- Delivery information is best-effort and may be missing or delayed.
- Delivery data is additive: it provides more context but never invalidates submission outcomes.
- Normalization must preserve meaning and avoid leaking provider-specific semantics to clients.

## Workflows (High-Level)

- Receipt ingestion: accept delivery confirmations from providers that support callbacks.
- Receipt reconciliation: map each delivery signal to the corresponding intent and update the delivery view.
- Client visibility: surface delivery state and history alongside, but separate from, submission status.
- Operator visibility: show delivery outcomes, delays, and gaps for operational diagnosis.

## Variations We Must Address

- Providers that push delivery receipts vs providers that only support polling.
- Providers that emit multiple delivery states vs a single final state.
- Duplicate or out-of-order receipts.
- Late receipts that arrive after the submission outcome is terminal.
- Missing receipts that never arrive.

## Race Conditions and Handling

- Receipts may arrive before a client observes acceptance; the system must converge to a stable delivery view regardless of observation order.
- Duplicate receipts must be safe and must not create conflicting delivery states.
- Out-of-order receipts must not produce inconsistent client-visible delivery history.

## Failure Semantics

- If receipt ingestion fails, submission outcomes remain authoritative and unchanged.
- If a provider does not supply receipts, delivery remains unknown rather than inferred.
- Incomplete delivery data must be explicitly represented as unknown or pending.

## Concurrency Guarantees

- Receipt processing is order-independent: the final delivery view must be consistent regardless of receipt arrival order.
- Concurrent receipts for the same intent must not produce divergent delivery states.

## Observable Acceptance Criteria

- Clients can query delivery status separately from submission status for a given intent.
- Operators can identify intents with missing or delayed receipts.
- Delivery data is visibly marked as best-effort and not authoritative for submission outcomes.

## Requires DESIGN decision

- Requires DESIGN decision: canonical correlation strategy for mapping provider delivery signals to intents.
- Requires DESIGN decision: delivery state taxonomy and the rules for state transitions.
