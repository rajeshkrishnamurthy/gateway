# Delivery Tracking V1 Slice: Service Topology

## Purpose / Big Picture

This slice defines the runtime deployment boundary for delivery tracking in V1. The intent is to remove coupling with SubmissionManager and to make delivery-tracking behavior independently deployable and horizontally scalable behind HAProxy.

This slice exists to make ownership explicit: delivery-tracking APIs and ingestion are owned by the delivery-tracking runtime, while SubmissionManager remains submission-only.

## Scope

This slice defines service ownership and request-routing topology for delivery-tracking surfaces in V1:

- `delivery read API` endpoints
- `provider signal webhook` ingress
- runtime scaling and failover expectations behind HAProxy

This slice also defines decoupling constraints between delivery-tracking runtime and SubmissionManager runtime.

## Non-goals

This slice does not define internal package structure, database schema details, webhook security hardening, or HAProxy configuration syntax.

This slice does not redefine delivery status semantics, delivery freshness semantics, or correlation semantics.

## Normative Inheritance

This slice inherits umbrella and foundational contracts from `specs/delivery-tracking/delivery-tracking-v1.md`, `specs/delivery-tracking/overview.md`, `specs/delivery-tracking/intent-correlation.md`, `specs/delivery-tracking/delivery-status-model.md`, `specs/delivery-tracking/delivery-freshness.md`, and `specs/delivery-tracking/ubiquitous-language.md`.

Unless this slice explicitly adds constraints, invariants, race handling, failure semantics, and concurrency guarantees are governed by those inherited documents.

## Topology Contract

For V1, delivery tracking must run as a dedicated deployable runtime that can be scaled to multiple instances. External clients and providers must call HAProxy frontends, and HAProxy must route requests to available delivery-tracking instances.

SubmissionManager must not host delivery-tracking endpoints or delivery-tracking webhook ingress. There is no backward-compatibility requirement for retaining delivery-tracking routes on SubmissionManager.

All cross-instance convergence must rely on persisted state and deterministic contracts, not on process-local memory or sticky routing.

## Invariants

Delivery-tracking endpoint ownership is exclusive: `delivery read API` and `provider signal webhook` ingress are served only by delivery-tracking runtime instances.

SubmissionManager remains submission-only and must not act as a compatibility proxy for delivery-tracking routes.

HAProxy-routed requests for delivery tracking must be functionally equivalent regardless of which healthy delivery-tracking instance handles the request.

## Race Conditions and Handling

Concurrent requests for the same intent may be routed to different delivery-tracking instances. The topology must preserve deterministic outcomes under this interleaving through shared persisted-state contracts.

Instance failure during active traffic must be handled by HAProxy failover/rerouting without introducing alternate semantics or requiring client-side route awareness.

## Failure Semantics

If no delivery-tracking instance is available, delivery-tracking requests fail at the HAProxy boundary according to standard gateway/unavailable behavior.

If one or more delivery-tracking instances remain healthy, partial instance outage must not require route changes by clients or providers.

SubmissionManager availability must not be a prerequisite for serving delivery-tracking routes that depend only on persisted delivery/submission state.

## Concurrency Guarantees

With multiple delivery-tracking instances, concurrent handling of equivalent requests over unchanged persisted state must yield equivalent responses and outcomes.

No sticky-session requirement is allowed for correctness. Correctness must hold under arbitrary HAProxy distribution across healthy instances.

## Observable Acceptance Criteria

Criterion 1: delivery-tracking runtime can run with at least two instances behind HAProxy, and both instances serve delivery-tracking routes.

Criterion 2: `delivery read API` and `provider signal webhook` routes are not served by SubmissionManager.

Criterion 3: stopping one delivery-tracking instance does not require client/provider route changes and does not alter delivery-tracking contract semantics.

Criterion 4: delivery-tracking responses for the same persisted snapshot are equivalent regardless of which healthy instance serves the request.

Criterion 5: no backward-compatibility route through SubmissionManager exists for delivery-tracking endpoints in V1.
