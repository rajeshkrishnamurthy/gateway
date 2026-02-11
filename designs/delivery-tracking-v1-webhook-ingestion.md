# Design: Delivery Tracking V1 Webhook Ingestion (Deterministic Conventions)

## Scope Guard
This design covers only `provider signal webhook` ingestion for delivery tracking V1: boundary validation, normalization, ingress-audit persistence, synchronous downstream invocation of correlation/core processing, and deterministic concurrency/error handling for ingestion. It does not design `provider signal poll`, delivery read API behavior, observability naming, or implementation code.

## Inputs (Normative Inheritance Set)
- `specs/delivery-tracking/delivery-tracking-v1-webhook-ingestion.md`
- `specs/delivery-tracking/delivery-tracking-v1-service-topology.md`
- `specs/delivery-tracking/delivery-tracking-v1.md`
- `specs/delivery-tracking/overview.md`
- `specs/delivery-tracking/intent-correlation.md`
- `specs/delivery-tracking/ubiquitous-language.md`
- `designs/delivery-tracking-v1-core-processing.md`
- `designs/adr/0001-delivery-tracking-durable-queueing.md`

## 1. Decision Statement
Choose one deterministic webhook-ingestion convention set so accepted `provider signal webhook` payloads are normalized, durably recorded for ingress audit, and synchronously applied through correlation/core semantics without introducing queue/inbox behavior in V1.

## 2. Options (1-3)

### Option A: Direct validate-and-apply with no ingress persistence
Short description:
- Validate and normalize request payload in memory, then immediately execute downstream correlation/core apply.
- No persisted ingress audit record.

Pros:
- Lowest write/storage overhead.
- Smallest runtime path.

Cons:
- No durable ingress trace for accepted requests.
- Harder post-incident diagnosis when downstream fails.

Risks / failure modes:
- Loss of accepted-ingress evidence if request succeeds at boundary but downstream fails.
- Reduced replay diagnostics.

Operational impact:
- Minimal database writes, lower diagnostics fidelity.

Compatibility with existing repo patterns:
- Medium-low. Weaker auditability than current SQL-first conventions.

### Option B: Persist ingress-audit record, then invoke correlation/core synchronously (Recommended)
Short description:
- Validate and normalize payload.
- Persist one ingress-audit record with generated `sourceRecordId` and deterministic timestamps.
- After ingress persistence commits, invoke correlation/core apply synchronously in request path using normalized payload plus `sourceRecordId`.
- No persisted handoff queue stage in V1.

Pros:
- Preserves accepted-ingress evidence for diagnostics.
- Keeps V1 aligned with explicit queueing deferral.
- Maintains deterministic downstream behavior using stable `sourceRecordId`.

Cons:
- Two-step processing boundary (ingress commit then downstream apply) requires explicit error mapping.
- Duplicate/replayed valid payloads still create additional ingress records by design.

Risks / failure modes:
- Ingress record can exist even when downstream apply fails in the same request path.
- Request latency includes downstream apply execution.

Operational impact:
- One ingress write transaction plus one downstream apply transaction per accepted payload.
- Better audit clarity than Option A.

Compatibility with existing repo patterns:
- High. Matches SQL durability for accepted boundary inputs and direct deterministic apply without queue workers.

### Option C: Persist ingress and process with asynchronous inbox/queue worker
Short description:
- Persist ingress records and process them asynchronously via worker claim/retry loops.

Pros:
- Better tolerance to transient downstream outages after ingress acceptance.
- Lower request latency variance.

Cons:
- Introduces queue/inbox lifecycle complexity that V1 explicitly defers.
- Requires retry, lease, dead-letter, and backlog semantics.

Risks / failure modes:
- Backlog/lag and worker coordination failures introduce new operational failure surfaces.
- Exactly-once reasoning burden increases.

Operational impact:
- Additional worker subsystem and monitoring.

Compatibility with existing repo patterns:
- Medium. Feasible, but outside V1 scope.

## 3. Recommendation (Exactly One)
Recommend **Option B**.

Rationale and tradeoffs:
- It retains ingress auditability while avoiding queue/inbox expansion that V1 defers.
- It keeps deterministic downstream semantics explicit by carrying `sourceRecordId` into correlation/core apply.
- Tradeoff accepted: ingress durability and downstream apply are separate boundaries, so ingress evidence may exist when downstream apply fails.

## 4. Approval Prompt (Exact Text)
Approve recommendation (Option B) for webhook-ingestion deterministic conventions.

## 5. Decision Record (Final Text)
Chosen option:
- Option B: persisted ingress-audit boundary plus synchronous downstream correlation/core apply, with no V1 queue/inbox stage.

### 5.0 Runtime ownership boundary
- `provider signal webhook` ingress is served only by delivery-tracking runtime instances behind HAProxy.
- SubmissionManager must not dual-serve or compatibility-proxy this ingress surface.
- Correctness must hold regardless of which healthy delivery-tracking runtime instance receives a webhook request.

### 5.1 Webhook normalization contract
- Accepted normalized payload must include:
  - `intentId` (required)
  - `providerDeliverySignal` (required)
  - `providerObservedAt` (optional)
  - `receivedAt` (required, Setu ingestion time)
  - source metadata identifying `provider signal webhook`
- Normalization is required before downstream processing. Correlation/core processing receives normalized fields, not raw provider payload shapes.

### 5.2 Validation and malformed timestamp handling
- Payload is invalid if required normalized fields are missing or structurally invalid (`intentId`, `providerDeliverySignal`).
- `providerObservedAt` is optional:
  - if absent: accept payload.
  - if present and valid timestamp: normalize to UTC and include.
  - if present but malformed/unparseable: do not reject payload; omit `providerObservedAt` and continue.
- `receivedAt` is always generated by Setu (database UTC time) for accepted payloads.

### 5.3 Idempotency and duplicate/replay handling
- Ingestion does not suppress duplicate/replayed valid payloads.
- Each valid payload instance generates one ingress `sourceRecordId` and one ingress-audit record.
- Duplicate/replay safety is guaranteed by deterministic downstream handling keyed by `sourceRecordId` semantics in core processing, not by webhook-ingestion suppression.
- Trusted-ingress-only in this round means replay protection controls are intentionally not part of this slice.

### 5.4 Timestamp precedence (`providerObservedAt` vs `receivedAt`)
- Deterministic precedence for downstream effective time:
  - Use `providerObservedAt` when present and valid.
  - Otherwise use `receivedAt`.
- Both values (when available) must be passed to downstream apply for deterministic ordering/tie-break rules.

### 5.5 Persistence and downstream-apply boundaries
- Boundary A (ingress acceptance):
  1. Begin transaction.
  2. Generate `receivedAt` from database UTC time.
  3. Generate stable `sourceRecordId`.
  4. Persist normalized ingress-audit record.
  5. Commit.
- Boundary B (downstream domain apply):
  - After Boundary A commit, invoke correlation/core apply synchronously in request path using normalized payload plus `sourceRecordId`.
  - Core-processing transaction atomicity for history + current-state mutation is governed by `designs/delivery-tracking-v1-core-processing.md`.
- V1 explicitly has no persisted correlation-handoff queue/inbox table as a processing stage.

### 5.6 Multi-instance concurrency and ordering behavior
- Any instance may process any `provider signal webhook` request.
- Ingestion does not guarantee arrival-order processing for the same `intentId`.
- Determinism requirements:
  - all instances use the same normalization rules.
  - all accepted payloads get `receivedAt` from database UTC time (not host local time).
  - downstream processing remains order-independent for duplicates and out-of-order signals, consistent with foundational contracts.

### 5.7 Error mapping (invalid payload vs transient internal failure)
- Invalid payload (required normalized fields invalid/missing): reject as client/input error (`400` class).
- Ingress persistence failure before Boundary A commit: fail request as transient server failure (`503` class).
- Downstream correlation/core unavailability or processing failure after Boundary A commit: fail request as transient server failure (`503` class); ingress-audit record remains as accepted-ingress evidence.
- Malformed `providerObservedAt` alone is not an invalid-payload failure; fallback behavior applies.

## Risks and Failure Modes (Chosen Option)
- Duplicate/replayed valid payloads can increase ingress write and downstream load.
- Ingress-audit records may exist for accepted payloads even when downstream apply fails in the same request.
- Database-time dependency means database availability directly affects ingress acceptance.

## Explicit Deferrals
- Webhook authentication is deferred.
- Signature verification is deferred.
- Replay protection controls are deferred.
- `provider signal poll` is deferred.
- Delivery read API contracts are deferred.
- Observability metric/log naming is deferred.
- Durable inbox/queue worker and asynchronous handoff processing are deferred in V1.

## ADR (Concise)
- ADR-INGEST-001: Use persisted ingress-audit boundary plus synchronous downstream correlation/core apply for accepted `provider signal webhook` payloads.
- ADR-INGEST-002: Treat malformed `providerObservedAt` as fallback-to-`receivedAt`, not hard reject.
- ADR-INGEST-003: Keep duplicate/replay suppression out of webhook-ingestion in V1; require deterministic downstream convergence.
- ADR-INGEST-004: Keep durable inbox/queue processing out of V1; revisit only in a future reliability phase.
