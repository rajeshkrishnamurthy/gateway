# Delivery Tracking: Delivery Freshness

## Purpose / Big Picture

This document defines the canonical freshness contract for delivery tracking in Setu. Freshness indicates whether unresolved delivery observations are still within expected observation conditions or have become stale. Freshness improves operational clarity without changing delivery status truth.
Canonical terminology for this module is defined in `specs/delivery-tracking/ubiquitous-language.md`.

## Scope

This spec covers only delivery freshness semantics for a single intent. It defines freshness states, freshness reclassification rules, and how freshness coexists with delivery status.

## Non-goals

This spec does not define delivery status taxonomy (`unknown`, `in_progress`, `delivered`, `failed`), provider mappings, ingestion mechanisms, storage schemas, or API payload details. It does not expose stale-to-fresh behavior as a public per-target config toggle.

## Freshness Intent

Freshness is a parallel dimension to delivery status. It communicates timeliness of unresolved delivery information. It does not assert whether delivery ultimately succeeded or failed.

The authoritative delivery status semantics are defined in `specs/delivery-tracking/delivery-status-model.md`.

## Contract Inputs

Delivery tracking settings are resolved from `submissionTarget` and snapshotted on the intent at submission time.

This freshness model applies only to intents whose snapshot has `deliveryTracking.mode` set to `on`.

If the snapshot has no `deliveryTracking` block, behavior is `mode=off`.

`deliveryTracking.staleAfterSeconds` is configured per target and is the public contract input for freshness staleness classification.

## Contract Input Validation

- `deliveryTracking.mode` must be either `on` or `off`.
- `deliveryTracking.staleAfterSeconds` is required when `mode=on`.
- `deliveryTracking.staleAfterSeconds` must be a positive integer.
- If `mode=off` and `staleAfterSeconds` is present, the value is ignored and a configuration warning is logged.

## Canonical Freshness States

- `fresh`: delivery status is unresolved and still within expected observation conditions.
- `stale`: delivery status is unresolved and has exceeded expected observation conditions without terminal delivery evidence.
- `not_applicable`: delivery status is terminal, so freshness no longer applies.

## Freshness Reclassification Rules

- `fresh` may transition to `stale` when unresolved delivery status exceeds expected observation conditions.
- `stale` may transition back to `fresh` only when a newer valid non-terminal provider delivery signal is observed.
- When a newer valid terminal provider delivery signal is observed, freshness transitions to `not_applicable`.
- Duplicate provider delivery signals or signals that are not considered newer must not reclassify `stale` to `fresh`.

## Invariants

- Freshness is additive and must not synthesize a terminal delivery status.
- Freshness must not change submission status or submission completion timestamps.
- Freshness must not change delivery status; it annotates timeliness of unresolved observations.
- Freshness is only meaningful for unresolved delivery status.
- For intents with delivery tracking `off`, freshness is not tracked.
- For intents with delivery tracking `on`, freshness uses the snapshotted `staleAfterSeconds` for that intent.
- Stale-to-fresh behavior is fixed semantics: a `stale` unresolved delivery status can move to `fresh` only on a newer valid non-terminal provider delivery signal.

## Race Conditions and Handling

Freshness reevaluation can race with incoming provider delivery signals. The model must converge deterministically regardless of operation order.

Out-of-order provider delivery signals must not cause oscillation that violates reclassification rules. Duplicate provider delivery signals must be benign and must not refresh stale freshness.

A stale unresolved delivery status may later receive terminal evidence. In that case, freshness becomes `not_applicable`.

## Failure Semantics

If freshness evaluation cannot run temporarily, the system must preserve last known freshness and must not infer terminal delivery status.

If expected observation conditions are unavailable, freshness must remain explicit and non-terminal, with no forced conversion to delivery failure.

Invalid contract config (`mode` invalid, missing `staleAfterSeconds` for `mode=on`, or non-positive `staleAfterSeconds`) fails registry/config load.

If `mode=off` includes `staleAfterSeconds`, runtime ignores the value and logs a configuration warning (non-fatal).

## Concurrency Guarantees

Concurrent freshness reevaluation and provider delivery signal processing must converge to a single consistent pair: delivery status plus delivery freshness.

Concurrent duplicate provider delivery signals must not produce divergent freshness states.

## Observable Acceptance Criteria

- For unresolved delivery status, observers can distinguish delivery freshness `fresh` versus `stale`.
- Stale freshness does not automatically convert delivery status to `failed`.
- Stale freshness can return to `fresh` only on newer valid non-terminal provider delivery signals.
- Terminal delivery status results in freshness `not_applicable`.
- For a target with no `deliveryTracking` block, delivery tracking behaves as `mode=off`.
- For intents with delivery tracking `on`, staleness is evaluated using that target's `staleAfterSeconds`.
- For `mode=off`, `staleAfterSeconds` (if present) is ignored and logged as configuration warning.
- Intents with delivery tracking `off` do not apply freshness classification.

## Requires DESIGN decision

- Requires DESIGN decision: deterministic timestamp basis used with `staleAfterSeconds` for staleness evaluation.
- Requires DESIGN decision: deterministic tie-break rule when candidate provider delivery signals have equal effective time.
- Requires DESIGN decision: deterministic definition of "newer valid signal" for freshness reclassification.
