# Design: Delivery Tracking V1 Service Topology (Lightweight)

## Scope Guard
This design covers only deterministic service-ownership and HAProxy topology conventions for delivery-tracking V1. It does not design delivery-status semantics, delivery-freshness semantics, correlation semantics, webhook security hardening, database schema details, or implementation code.

## Inputs
- `specs/delivery-tracking/delivery-tracking-v1-service-topology.md`
- `specs/delivery-tracking/delivery-tracking-v1.md`
- `specs/delivery-tracking/overview.md`
- `specs/delivery-tracking/intent-correlation.md`
- `specs/delivery-tracking/delivery-status-model.md`
- `specs/delivery-tracking/delivery-freshness.md`
- `specs/delivery-tracking/ubiquitous-language.md`

## 1. Decision Statement
Standardize one deterministic topology convention so delivery-tracking route ownership, multi-instance HAProxy behavior, and SubmissionManager decoupling are unambiguous for EXEC.

## 2. Decision Record (Final Text)
Chosen option:
- Use dedicated delivery-tracking runtime ownership for `delivery read API` and `provider signal webhook`, served behind HAProxy with multi-instance correctness independent of sticky routing.

Convention and mapping definition:

1) Ownership boundary
- `delivery read API` is owned only by delivery-tracking runtime.
- `provider signal webhook` ingress is owned only by delivery-tracking runtime.
- SubmissionManager remains submission-only and must not dual-serve or compatibility-proxy delivery-tracking routes.

2) Routing boundary
- External clients and providers call HAProxy frontends for delivery-tracking surfaces.
- HAProxy routes requests to healthy delivery-tracking runtime instances.
- Route correctness must not depend on sticky-session behavior.

3) Multi-instance determinism
- With two or more healthy delivery-tracking runtime instances, equivalent requests over unchanged persisted state must yield equivalent responses/outcomes.
- Cross-instance convergence relies on shared persisted-state contracts and deterministic rules, not process-local memory.

4) Failure behavior at topology boundary
- If one or more delivery-tracking runtime instances are healthy, partial instance outage must not require route changes by clients/providers.
- If no delivery-tracking runtime instance is available, delivery-tracking requests fail at HAProxy unavailable boundary behavior.
- SubmissionManager availability is not a prerequisite for serving delivery-tracking routes that depend only on persisted delivery/submission state.

Configuration parameters:
- No new public per-target configuration is introduced.
- Deployment/runtime topology settings are operational concerns outside client contract fields.

Constraints that must hold:
- No backward-compatibility route through SubmissionManager exists for delivery-tracking endpoints in V1.
- Correctness must hold under arbitrary HAProxy distribution across healthy delivery-tracking runtime instances.

## 3. Brief Rationale
This codifies the service boundary introduced by SPEC so EXEC can implement runtime separation without inference. It keeps correctness anchored to persisted deterministic contracts and prevents accidental fallback coupling to SubmissionManager.

## ADR (Concise)
- ADR-TOPO-001: Delivery-tracking route ownership is exclusive to dedicated delivery-tracking runtime in V1.
- ADR-TOPO-002: HAProxy sticky-session behavior is not a correctness requirement for delivery-tracking routes.

## Explicit Deferrals
- No HAProxy configuration syntax is designed here.
- No webhook auth/signature/replay protection controls are designed here.
- No internal package-structure decisions are designed here.
