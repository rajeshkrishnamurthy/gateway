# Delivery Tracking V1 (Umbrella)

## Purpose / Big Picture

This document exists to make V1 implementation planning predictable without redefining domain semantics that are already established. The intent is to create one composition boundary for delivery tracking so EXEC and VERIFY phases can work from a stable contract that clearly states why V1 exists, what V1 must deliver, and what must be resolved before work is marked EXEC-ready.

This umbrella spec is intentionally compositional. It does not replace foundational delivery-tracking specs and does not introduce alternate semantics.

Canonical terminology for this module is defined in `specs/delivery-tracking/ubiquitous-language.md`.

## Scope

V1 includes the complete delivery-tracking path from contract gating through observation visibility. That path starts with `submissionTarget` snapshot settings (`deliveryTracking.mode` and `deliveryTracking.staleAfterSeconds`), continues through provider delivery signal ingestion and signal-to-intent correlation using `intentId`, and applies canonical delivery-status and delivery-freshness semantics. V1 also includes client and operator visibility of delivery status, delivery freshness, and delivery history as a dimension that is explicitly separate from submission status, along with audit visibility for ignored signals when tracking is `off`.

V1 delivery tracking must be deployable as a dedicated multi-instance runtime behind HAProxy. Delivery-tracking endpoint ownership is independent from SubmissionManager, and clients/providers call HAProxy rather than addressing runtime instances directly.

In this DESIGN/EXEC round, webhook ingestion is functionally scoped and assumes trusted ingress. Webhook authentication, signature verification, and replay protection are intentionally deferred.

## Non-goals

V1 does not change submission status or submission completion timestamps, does not add new public per-target configuration beyond `deliveryTracking.mode` and `deliveryTracking.staleAfterSeconds`, and does not expose provider-specific semantics at the contract boundary. V1 also does not define delivery guarantees or SLAs. Any implementation that weakens foundational invariants is out of scope for V1.

Webhook security hardening is out of scope for this round. Specifically, authentication, signature verification, and replay protection for incoming delivery webhooks are deferred and must be completed before production internet exposure.

Backward-compatibility routing that keeps delivery-tracking endpoints on SubmissionManager is out of scope for V1.

## Foundational Specs (Normative)

The normative inputs for this umbrella are `specs/delivery-tracking/overview.md`, `specs/delivery-tracking/delivery-status-model.md`, `specs/delivery-tracking/delivery-freshness.md`, `specs/delivery-tracking/intent-correlation.md`, and `specs/delivery-tracking/ubiquitous-language.md`. This umbrella is a composition boundary for V1, not a semantic duplicate of those documents. Slice specs must reference these foundations directly instead of restating behavior.

## Invariants

This umbrella is composition-only and must not redefine delivery semantics that are already normative in the foundational specs.

V1 slice specs must preserve, without weakening, the invariants defined in:

- `specs/delivery-tracking/overview.md`
- `specs/delivery-tracking/delivery-status-model.md`
- `specs/delivery-tracking/delivery-freshness.md`
- `specs/delivery-tracking/intent-correlation.md`

## Race Conditions and Handling

Each V1 slice spec must explicitly state race handling for its surface area and must reference the applicable foundational race semantics instead of restating or modifying them.

If a slice introduces a new race condition not covered by foundational specs, SPEC must update foundational docs first (or in the same SPEC session) before the slice can be marked complete.

## Failure Semantics

Each V1 slice spec must define failure behavior for its own boundary and must remain consistent with foundational failure semantics.

No V1 slice is allowed to reinterpret foundational failure outcomes (for example by changing immutable submission status or by converting freshness staleness into terminal delivery failure).

For this round, webhook ingestion must be treated as functional-only and not production-ready. Public internet exposure without the deferred webhook security controls is not allowed.

## Concurrency Guarantees

Each V1 slice spec must include concurrency guarantees relevant to its responsibility and must be compatible with the foundational determinism contract across delivery status and delivery freshness.

No slice is allowed to bypass contract gating or introduce nondeterministic update behavior.

## Observable Acceptance Criteria

V1 umbrella acceptance is satisfied only when these spec-level conditions are true.

Criterion 1: all required V1 slice specs listed below exist under `specs/delivery-tracking/`.

Criterion 2: each required slice spec references foundational specs as normative sources and does not duplicate core semantic definitions.

Criterion 3: each required slice spec includes observable acceptance criteria that can be mapped to foundational contracts without ambiguity.

Criterion 4: combined slice coverage spans the complete V1 path of webhook ingestion, intent correlation, core processing, delivery read API visibility, and observability/audit.

Criterion 5: unresolved deterministic conventions are explicitly listed in `Requires DESIGN decision`, with no hidden implementation choices.

Criterion 6: the webhook-ingestion slice explicitly states trusted-ingress-only operation for this round and explicitly marks production internet exposure as blocked until deferred webhook security controls are specified and implemented.

Criterion 7: V1 slice set explicitly defines dedicated delivery-tracking service topology, HAProxy multi-instance routing expectations, and decoupled ownership from SubmissionManager.

## Required V1 Slice Specs

Before EXEC planning, SPEC must produce the following slice specs:

1. `specs/delivery-tracking/delivery-tracking-v1-core-processing.md`
2. `specs/delivery-tracking/delivery-tracking-v1-webhook-ingestion.md`
3. `specs/delivery-tracking/delivery-tracking-v1-get-retrieval.md`
4. `specs/delivery-tracking/delivery-tracking-v1-observability.md`
5. `specs/delivery-tracking/delivery-tracking-v1-service-topology.md`

Each slice spec must define its own scope, non-goals, invariants, race handling, failure semantics, concurrency guarantees, and observable acceptance criteria, while referencing foundational specs for shared semantics.

The webhook-ingestion slice must explicitly document the temporary security deferral and the production-exposure guardrail for this round.

## Requires DESIGN decision

DESIGN must select deterministic conventions for the timestamp basis and timestamp precedence used during freshness staleness evaluation against `staleAfterSeconds`.

DESIGN must define a deterministic tie-break rule when candidate provider delivery signals have equal effective time.

DESIGN must define the deterministic meaning of `newer valid signal` used for stale-to-fresh reclassification.

DESIGN must define deterministic terminal conflict handling if both terminal delivery polarities are observed for the same intent.

## V1 Slice Planning Boundary

Slice specs must be created under `specs/delivery-tracking/` and must reference foundational specs rather than duplicating semantics.

This umbrella spec is not EXEC-ready by itself. EXEC readiness requires DESIGN-mode resolution of required deterministic conventions and concrete V1 slice specs with observable acceptance criteria per slice.
