# Design: Delivery Tracking V1 Delivery Read API (Lightweight)

## Scope Guard
This design covers only deterministic retrieval conventions for the `delivery read API` slice in `specs/delivery-tracking/delivery-tracking-v1-get-retrieval.md`. It does not design provider signal ingestion, delivery core-processing semantics, observability naming, or implementation code.

## Inputs
- `specs/delivery-tracking/delivery-tracking-v1-get-retrieval.md`
- `specs/delivery-tracking/delivery-tracking-v1-service-topology.md`
- `specs/delivery-tracking/delivery-tracking-v1.md`
- `specs/delivery-tracking/overview.md`
- `specs/delivery-tracking/delivery-status-model.md`
- `specs/delivery-tracking/delivery-freshness.md`
- `specs/delivery-tracking/ubiquitous-language.md`
- `designs/delivery-tracking-v1-core-processing.md`

## 1. Decision Statement
Standardize one deterministic read-materialization convention so both delivery read API endpoints return stable, canonical payloads under concurrent writes and sparse tracked-intent delivery data.

## 2. Decision Record (Final Text)
Chosen option:
- Use single-snapshot read materialization per endpoint request with explicit mode-gated response shaping and deterministic fallback for sparse tracked-intent delivery data.

Convention and mapping definition:

1) Runtime ownership and routing boundary
- `delivery read API` endpoints are served only by delivery-tracking runtime instances behind HAProxy.
- SubmissionManager must not dual-serve or compatibility-proxy these endpoints.
- Correctness must hold under arbitrary HAProxy distribution across healthy delivery-tracking runtime instances; sticky-session behavior is not required for correctness.

2) Endpoint materialization boundary
- Each endpoint request materializes response data from one database snapshot boundary (single transaction/snapshot read), so response fields are internally consistent.
- No endpoint may combine data from different snapshots for the same response.

3) `GET /v1/intents/{intentId}/delivery` response shaping
- Resolve intent first, including snapshotted `delivery tracking mode` and `staleAfterSeconds`.
- If intent is unknown: return not-found semantics aligned with existing submission APIs.
- If `delivery tracking mode=off`: return only `intentId` and `deliveryTrackingMode=off`; do not return synthesized `delivery status` or `delivery freshness`.
- If `delivery tracking mode=on`:
  - Preferred source is persisted current delivery projection from core processing.
  - If projection exists: return canonical `deliveryStatus` and `deliveryFreshness` from that projection.
  - If projection is sparse/missing: return canonical fallback values:
    - `deliveryStatus=unknown`
    - `deliveryFreshness` computed from unresolved staleness conditions using snapshotted `staleAfterSeconds` and intent creation time (`fresh` before threshold, `stale` at/after threshold).

4) `GET /v1/intents/{intentId}/delivery/history` response shaping
- Resolve intent first.
- If intent is unknown: return not-found semantics aligned with existing submission APIs.
- If `delivery tracking mode=off`: return `intentId`, `deliveryTrackingMode=off`, and `entries=[]`.
- If `delivery tracking mode=on`:
  - Return `entries` as append-only ordered list from oldest to newest.
  - Ordering must be by persisted append order (stable history sequence), not ad hoc timestamp sorting.
  - Each entry includes at least `recordedAt`, `providerDeliverySignal`, `deliveryStatus`, and `lateObservation`.

5) Concurrency and race behavior for reads
- Concurrent reads over unchanged persisted state must return equivalent payloads.
- Reads racing with writes may observe either pre-write or post-write snapshot, but not mixed partial state in one response.
- Retrieval must remain deterministic even when provider delivery signals arrive out of order; read ordering is always by persisted history append order.

6) Failure behavior and error mapping
- Unknown intent: not-found mapping consistent with existing submission APIs.
- Materialization unavailable (storage/query failure): internal error mapping consistent with existing submission APIs; do not return partial malformed payloads.
- Sparse tracked-intent delivery data is not an automatic internal error; canonical fallback shaping applies as above.

Configuration parameters:
- No new public configuration is introduced.
- Existing snapshotted `delivery tracking mode` and `staleAfterSeconds` are the only delivery-read shaping inputs from contract state.

Constraints that must hold:
- Retrieval is read-only and must not mutate `submission status`, `delivery status`, `delivery freshness`, or `delivery history`.
- Mode-off behavior must never fabricate `delivery status`/`delivery freshness`.
- History ordering must be stable and deterministic for the same persisted snapshot.

## 3. Brief Rationale
This keeps EXEC deterministic without adding architectural scope: one snapshot boundary, one response-shaping rule per mode, and one stable history-order rule. It also keeps consistency with core-processing design by preferring persisted projection while still honoring the spec requirement to return canonical representation when tracked-intent delivery data is sparse.

## ADR (Concise)
- ADR-READ-001: Use single-snapshot read materialization per endpoint response to prevent mixed-state payloads.
- ADR-READ-002: Use persisted append order as the canonical history ordering source.
- ADR-READ-003: For tracked intents with sparse delivery projection, return canonical fallback (`deliveryStatus=unknown`, freshness from staleness conditions) instead of malformed partial payloads.

## Explicit Deferrals
- No provider signal ingestion or correlation behavior is designed here.
- No delivery core-processing transition logic is designed here.
- No observability metric/log naming is designed here.
- No pagination/retention policy changes are designed here.
