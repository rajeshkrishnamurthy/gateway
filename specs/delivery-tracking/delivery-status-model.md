# Delivery Tracking: Delivery Status Model

## Purpose / Big Picture

This document defines the canonical delivery-status contract for Setu. It specifies how delivery status is represented over time, independent of provider-specific vocabulary and independent of submission status. The goal is a stable, deterministic delivery view that remains meaningful even when signals are delayed, duplicated, or incomplete.
Canonical terminology for this module is defined in `specs/delivery-tracking/ubiquitous-language.md`.

## Scope

This spec covers the semantic model of delivery status and delivery history for a single intent. It defines what delivery status values exist, what those values mean, and how delivery status transitions must be interpreted at the domain level.

## Non-goals

This spec does not define ingestion mechanisms, persistence schemas, API payload structures, or provider adapters. It also does not redefine submission semantics or introduce new submission status values. It does not expose correlation or conflict-resolution knobs as public per-target config.

## Delivery Status Intent

Delivery status is a separate dimension from submission status. Submission status answers whether Setu successfully submitted according to contract policy. Delivery status answers what was later observed about downstream delivery, if anything was observed.

The delivery model must support both a current-delivery-status view and an event-history view so that clients and operators can understand not only where an intent stands now, but also how it reached that point.

## Contract Gating

Delivery tracking settings are resolved from `submissionTarget` and snapshotted on the intent at submission time.

This delivery-status model applies only when that snapshot has `deliveryTracking.mode` set to `on`.

If the snapshot has no `deliveryTracking` block, behavior is `mode=off`.

If the snapshot has `deliveryTracking.mode=off`, provider delivery signal processing does not update delivery status. Incoming provider delivery signals are still logged as ignored (`mode_off`) for audit and troubleshooting.

## Canonical Delivery Status Values

The delivery layer must support the following canonical delivery status values:

- `unknown`: no authoritative delivery signal has been observed yet.
- `in_progress`: at least one non-terminal delivery progress signal has been observed.
- `delivered`: a terminal success delivery signal has been observed.
- `failed`: a terminal failure delivery signal has been observed.

The model must also preserve whether a terminal delivery signal was observed after submission had already reached terminal submission status. That condition is represented as a late delivery observation in delivery history and must remain visible to clients and operators.

Freshness semantics are defined separately in `specs/delivery-tracking/delivery-freshness.md`. Delivery status and delivery freshness are parallel dimensions.

## Status Progression Intent

Delivery status progression represents observation over time, not a guarantee timeline. A later provider delivery signal can provide more complete information, but must not rewrite submission status.

At the domain level:

- `unknown` may transition to `in_progress`, `delivered`, or `failed`.
- `in_progress` may transition to `delivered` or `failed`.
- Once `delivered` or `failed` is reached, the delivery status is terminal for client-facing current delivery status.

History remains append-only in meaning: every accepted, correlated provider delivery signal contributes to the explanation of current delivery status, even when the current delivery status is already terminal.

Terminal conflict handling is deterministic. The first observed terminal delivery status (`delivered` or `failed`) locks the current delivery status. Any later opposite terminal observation must remain visible in delivery history, but must not change the locked current delivery status.

## Invariants

- Delivery status is independent from submission status and does not modify submission status.
- A late terminal delivery success after terminal submission status does not change submission status; it is recorded as a late delivery success observation.
- Delivery status must be provider-agnostic at the contract boundary.
- Current delivery status must always be derivable from delivery history under deterministic rules.
- Delivery status progression is independent from delivery freshness classification.
- For intents with delivery tracking `off`, delivery status is not tracked and does not transition.
- Correlation fallback behavior, ambiguity handling, and terminal conflict handling are internal deterministic semantics and are not public per-target config.
- After terminal lock, opposite terminal observations are history-only and must not mutate current delivery status.

## Race Conditions and Handling

Provider delivery signals may arrive out of order, concurrently, or after long delay. The model must converge to a coherent current delivery status regardless of signal arrival sequence.

Duplicate provider delivery signals must be treated as non-conflicting observations. They may enrich history but must not create contradictory current delivery status outcomes.

Late signals must remain visible in history so observers can distinguish timely delivery observations from delayed ones.

If opposite terminal observations race or arrive out of order, the model must still converge to one locked current terminal delivery status while preserving the opposite observation in delivery history.

## Failure Semantics

If provider delivery signals are absent, delivery status remains `unknown`; no inferred terminal delivery status is allowed.

If provider delivery signals are inconsistent, the model must preserve determinism by favoring stable canonical interpretation rules rather than provider-specific ad hoc assumptions.

If delivery processing is temporarily unavailable, submission status remains authoritative and unaffected.

## Concurrency Guarantees

Concurrent signal processing for the same intent must converge to the same current delivery status and compatible history semantics, independent of processing order.

Concurrent duplicates must not lead to divergent current statuses.

## Observable Acceptance Criteria

- For any intent, observers can see submission status and delivery status as separate dimensions.
- Delivery status can be `unknown`, `in_progress`, `delivered`, or `failed`.
- Observers can identify when a delivery terminal signal was late relative to submission terminalization.
- A late `delivered` observation after submission `rejected` or `exhausted` keeps submission status unchanged while updating delivery history and current delivery status.
- If both terminal outcomes are observed, the first terminal outcome remains the current delivery status and the opposite terminal observation remains visible in delivery history.
- For a target with no `deliveryTracking` block, delivery tracking behaves as `mode=off`.
- Intents with delivery tracking `off` do not transition through delivery statuses.
- For intents with delivery tracking `off`, provider delivery signals are logged as ignored (`mode_off`) for audit and troubleshooting.
