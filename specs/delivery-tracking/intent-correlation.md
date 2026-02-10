# Delivery Tracking: Intent Correlation

## Purpose / Big Picture

This document defines how an incoming provider delivery signal is correlated to a single SubmissionManager intent before any delivery status or delivery freshness changes are applied. Correlation is the correctness boundary that prevents signal application to the wrong intent.
Canonical terminology for this module is defined in `specs/delivery-tracking/ubiquitous-language.md`.

## Scope

This spec covers only the domain contract for signal-to-intent correlation:

- canonical correlation key
- correlation results
- conditions under which downstream delivery status and delivery freshness changes are allowed

## Non-goals

This spec does not define provider adapter payload formats, persistence schemas, API payload shapes, or retry/replay mechanics. It does not expose correlation controls as public per-target config.

## Correlation Intent

Correlation is a mandatory gate before delivery status and delivery freshness processing. A provider delivery signal may affect delivery semantics only when correlation succeeds for exactly one intent.

## Contract Gating

Correlation is applied only for intents whose resolved `submissionTarget` contract snapshot has `deliveryTracking.mode` set to `on`. If delivery tracking is `off`, provider delivery signals are not correlated for that intent.

## Canonical Correlation Key

The canonical correlation key is `intentId`.

Correlation uses exact `intentId` matching against existing intents. This key is deterministic and unique by intent contract.

## Correlation Results

- `matched`: the signal contains a valid `intentId` and exactly one intent exists for that key.
- `unmatched`: the signal contains a valid `intentId` but no intent exists for that key.
- `invalid`: the signal does not provide a valid canonical correlation key.

By contract, ambiguous result is not expected because correlation uses a single unique key (`intentId`).

## Invariants

- Correlation is deterministic: the same signal key against the same intent set yields the same correlation result.
- Delivery status and delivery freshness changes are allowed only for `matched` signals.
- `unmatched` and `invalid` signals must not mutate delivery status, delivery freshness, or submission status.
- Correlation behavior is internal deterministic semantics and is not public per-target config.
- Correlation never changes submission status.

## Race Conditions and Handling

Provider delivery signals may arrive out of order, be duplicated, or arrive long after submission terminalization. Correlation result for each signal is computed independently at processing time using canonical key lookup.

A signal that is `unmatched` at one time may be followed by a later signal that becomes `matched` once the corresponding intent is present; each signal is evaluated independently without rewriting earlier outcomes.

Duplicate signals for the same canonical key must correlate consistently and must not produce conflicting intent associations.

## Failure Semantics

If correlation processing is unavailable, delivery status and delivery freshness changes are not applied.

If canonical key extraction is invalid, the signal is `invalid` and no delivery status or delivery freshness change is applied.

If key lookup fails transiently, no delivery status or delivery freshness change is applied until correlation can be evaluated deterministically.

## Concurrency Guarantees

Concurrent correlation processing for signals with the same canonical key must converge on the same correlation result under the same observed intent set.

No signal may be correlated to more than one intent.

## Observable Acceptance Criteria

- A signal with valid `intentId` matching an existing intent is classified `matched` and is eligible for downstream delivery status and delivery freshness processing.
- A signal with valid `intentId` that does not match any intent is classified `unmatched` and does not change delivery status or delivery freshness.
- A signal without a valid canonical key is classified `invalid` and does not change delivery status or delivery freshness.
- Repeated signals with the same valid key do not produce different intent associations.
- Correlation processing never changes submission status.
