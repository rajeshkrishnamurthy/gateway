# Delivery Tracking V1 Slice: Webhook Ingestion

## Purpose / Big Picture

This slice defines how provider delivery signals enter Setu through `provider signal webhook` ingress in the current DESIGN/EXEC round. The purpose is to establish a deterministic ingestion boundary that feeds correlation and core processing without coupling domain behavior to provider payload shapes.

This slice is functional-only for security in the current round and must be treated as non-production-ready for public internet exposure.

## Scope

This slice covers provider-to-Setu push ingress for provider delivery signals, including boundary validation, normalization into an internal handoff contract, and deterministic handoff to correlation. Ingress in this slice is webhook-only. `provider signal poll` is explicitly out of scope in this round. Gateway/provider adapters may expose provider-specific webhook payload shapes, but every accepted payload must be normalized before entering correlation.

Webhook ingress in this slice is owned by delivery-tracking runtime instances behind HAProxy and is not hosted by SubmissionManager.

## Non-goals

This slice does not define `provider signal poll` behavior, delivery-status progression rules, delivery-freshness progression rules, delivery read API contracts, or full webhook security hardening. This slice also does not define provider-specific business semantics as public contract behavior.

This slice does not provide backward-compatible provider-signal webhook hosting through SubmissionManager.

## Normative Inheritance

This slice inherits umbrella and foundational contracts from `specs/delivery-tracking/delivery-tracking-v1.md`, `specs/delivery-tracking/overview.md`, `specs/delivery-tracking/intent-correlation.md`, and `specs/delivery-tracking/ubiquitous-language.md`.

Unless this slice explicitly adds constraints, invariants, race handling, failure semantics, and concurrency guarantees are governed by those inherited documents.

## Ingestion Handoff Contract

Before correlation, each accepted webhook payload must be normalized into one internal handoff record that includes at least `intentId`, `providerDeliverySignal`, `providerObservedAt` (optional), and `receivedAt` (required Setu ingestion time). Additional provider metadata may be included for audit and debugging, but it must not bypass canonical correlation rules.

## Invariants

Only normalized records with required fields are eligible for downstream correlation. Invalid payloads are rejected before correlation handoff. Ingress source identity is preserved as `provider signal webhook` in downstream audit/observability metadata.

`provider signal webhook` ingress endpoint ownership is exclusive to delivery-tracking runtime and must not be dual-served by SubmissionManager.

## Race Conditions and Handling

Normalization and handoff remain deterministic under concurrent, duplicated, and out-of-order inbound webhook payloads, so downstream correlation and core processing converge without ambiguity.

## Failure Semantics

Validation failure rejects normalization and emits invalid-ingestion visibility. Downstream handoff unavailability fails the webhook request without partial handoff. Missing or invalid `providerObservedAt` falls back to `receivedAt` availability for downstream deterministic time handling.

## Concurrency Guarantees

Concurrent ingestion must not produce conflicting normalized records for the same payload body and ingest context, and no path may bypass required normalization fields before correlation.

## Security Deferral Guardrail (Current Round)

Webhook authentication, signature verification, and replay protection are intentionally deferred in this round, as established by `specs/delivery-tracking/delivery-tracking-v1.md`. This slice must therefore be treated as trusted-ingress-only and must not be internet-exposed in production until deferred webhook security controls are specified and implemented.

## Observable Acceptance Criteria

Criterion 1: provider webhook payloads with required fields are accepted for normalization and passed to correlation input.

Criterion 2: payloads missing required normalized fields are rejected and do not change delivery status or delivery freshness.

Criterion 3: ingestion output includes source metadata identifying `provider signal webhook`.

Criterion 4: `provider signal poll` is not implemented in this slice.

Criterion 5: the slice explicitly documents trusted-ingress-only operation and blocks production internet exposure until webhook security hardening is completed.

Criterion 6: provider delivery webhook ingress is served by delivery-tracking runtime behind HAProxy and is not served by SubmissionManager.
