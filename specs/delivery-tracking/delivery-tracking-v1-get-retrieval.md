# Delivery Tracking V1 Slice: Delivery Read API

## Purpose / Big Picture

This slice defines client-facing retrieval of delivery tracking results through the `delivery read API`. The purpose is to provide clear, canonical, and submission-independent read contracts for delivery status, delivery freshness, and delivery history.

This slice uses Option B separation: one endpoint for current delivery status/freshness and one endpoint for delivery history.

## Scope

This slice defines two delivery read API endpoints: `GET /v1/intents/{intentId}/delivery` and `GET /v1/intents/{intentId}/delivery/history`. These endpoints are delivery-focused contracts and must not require clients to parse submission endpoints to obtain delivery tracking results.

## Non-goals

This slice does not define provider-facing ingress contracts, delivery status progression rules, delivery freshness progression rules, or storage schema internals.

This slice does not change existing submission GET response contracts.

## Normative Inheritance

This slice inherits umbrella and foundational contracts from `specs/delivery-tracking/delivery-tracking-v1.md`, `specs/delivery-tracking/overview.md`, `specs/delivery-tracking/delivery-status-model.md`, `specs/delivery-tracking/delivery-freshness.md`, and `specs/delivery-tracking/ubiquitous-language.md`.

Unless this slice explicitly adds constraints, invariants, race handling, failure semantics, and concurrency guarantees are governed by those inherited documents.

## Endpoint Contracts

### `GET /v1/intents/{intentId}/delivery`

This endpoint returns the current delivery-tracking view for the intent. If the intent exists and `delivery tracking mode` is `on`, the response must include `intentId`, `deliveryTrackingMode=on`, `deliveryStatus`, and `deliveryFreshness`. If the intent exists and `delivery tracking mode` is `off`, the response must include `intentId` and `deliveryTrackingMode=off`. In mode `off`, `deliveryStatus` and `deliveryFreshness` must not be returned as synthesized values.

### `GET /v1/intents/{intentId}/delivery/history`

This endpoint returns delivery history for the intent as an append-only ordered list. If the intent exists and `delivery tracking mode` is `on`, the response must include `intentId`, `deliveryTrackingMode=on`, and `entries` ordered from oldest to newest. Each entry must include at least `recordedAt`, `providerDeliverySignal`, `deliveryStatus`, and `lateObservation`. If the intent exists and `delivery tracking mode` is `off`, the response must include `intentId`, `deliveryTrackingMode=off`, and `entries` as an empty array.

## Error Mapping

If `intentId` is unknown, both endpoints return not found semantics consistent with existing submission APIs.

Unexpected retrieval failures return internal error semantics consistent with existing submission APIs.

## Invariants

Slice-specific read-contract invariants are read-only retrieval, explicit mode-off representation with no fabricated delivery status or delivery freshness values, and stable delivery-history ordering for the same persisted snapshot.

## Race Conditions and Handling

Slice-specific read race handling is snapshot consistency at endpoint boundaries: a response must not mix incompatible snapshots, and history ordering guarantees must hold even when reads race with concurrent writes.

## Failure Semantics

Slice-specific failure behavior is deterministic endpoint failure without partial malformed payloads when read materialization is unavailable. If tracked-intent delivery data is sparse, representation must still follow canonical delivery status and delivery freshness contracts.

## Concurrency Guarantees

Slice-specific guarantee is retrieval determinism: repeated reads over unchanged persisted state return the same payload, and concurrent reads for the same persisted snapshot do not produce structurally incompatible contracts.

## Observable Acceptance Criteria

Criterion 1: `GET /v1/intents/{intentId}/delivery` and `GET /v1/intents/{intentId}/delivery/history` both exist and are delivery-only contracts.

Criterion 2: for tracked intents, the current endpoint exposes canonical `deliveryStatus` and `deliveryFreshness`.

Criterion 3: for tracked intents, the history endpoint returns append-only ordered entries with required fields.

Criterion 4: for mode-off intents, both endpoints return `deliveryTrackingMode=off` and do not fabricate delivery status or delivery freshness.

Criterion 5: unknown `intentId` yields not-found semantics consistent with existing submission APIs.
