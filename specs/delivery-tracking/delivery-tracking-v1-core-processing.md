# Delivery Tracking V1 Slice: Core Processing

## Purpose / Big Picture

This slice defines the runtime core that turns correlated provider delivery signals into canonical delivery status, delivery freshness, and delivery history for an intent. The purpose is to make the domain progression path deterministic and isolated from transport concerns.

This slice composes foundational behavior and does not redefine it.

## Scope

This slice starts after a provider delivery signal has been accepted by ingestion and correlation has produced a `correlation result`. For `matched` signals on intents whose `delivery tracking mode` is `on`, this slice applies delivery-status progression, delivery-freshness progression, and delivery-history append semantics. This slice also includes mode-off gating behavior at the processing boundary.

## Non-goals

This slice does not define provider-facing webhook payload formats, `provider signal poll` behavior, storage schema details, delivery read API response shapes, or metrics naming. This slice does not change submission status semantics.

## Normative Inheritance

This slice inherits umbrella and foundational contracts from `specs/delivery-tracking/delivery-tracking-v1.md`, `specs/delivery-tracking/overview.md`, `specs/delivery-tracking/delivery-status-model.md`, `specs/delivery-tracking/delivery-freshness.md`, `specs/delivery-tracking/intent-correlation.md`, and `specs/delivery-tracking/ubiquitous-language.md`.

Unless this slice explicitly adds constraints, invariants, race handling, failure semantics, and concurrency guarantees are governed by those inherited documents.

## Invariants

Only `matched` signals are eligible for domain application at the core-processing boundary. Mode-off processing remains a no-op for delivery status and delivery freshness while preserving `mode_off` audit visibility. Delivery history remains append-only in meaning so current delivery status is derivable deterministically. For matched tracked intents, idempotent delivery-history write and any current delivery state mutation must be committed in one atomic unit.

## Race Conditions and Handling

Duplicated, out-of-order, and late provider delivery signals at the core-processing boundary must converge to one deterministic delivery status plus delivery freshness result. Late terminal delivery observations must remain visible in delivery history through this path.

## Failure Semantics

Core application is atomic: if a signal cannot be applied deterministically, partial mutation is not allowed and delivery status plus delivery freshness remain at last valid values. Atomicity includes idempotent delivery-history write and current-state mutation in the same commit boundary, with no partial commit allowed between them. If freshness evaluation is unavailable at application time, last known freshness is preserved at the core boundary.

## Concurrency Guarantees

Identical persisted intent state, delivery history, and accepted signal order yield identical core-processing outputs, with no concurrent bypass of contract or correlation gating in this path.

## Observable Acceptance Criteria

Criterion 1: for intents with `delivery tracking mode` set to `off`, core processing does not change delivery status or delivery freshness, and ignored-signal audit visibility is preserved as `mode_off`.

Criterion 2: for `correlation result` values `unmatched` or `invalid`, core processing does not change delivery status or delivery freshness.

Criterion 3: for `matched` signals on tracked intents, core processing updates delivery status and delivery history in accordance with `specs/delivery-tracking/delivery-status-model.md`.

Criterion 4: for tracked intents, stale-to-fresh reclassification occurs only on a newer valid non-terminal provider delivery signal, consistent with `specs/delivery-tracking/delivery-freshness.md`.

Criterion 5: late delivery success after terminal submission status is visible in delivery history and current delivery status while submission status remains unchanged.

Criterion 6: repeated processing under identical persisted inputs yields identical delivery status and delivery freshness.

Criterion 7: for matched tracked intents, idempotent history persistence and current-state mutation are committed atomically with no partial commit boundary between them.
