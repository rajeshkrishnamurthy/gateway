# Design: Delivery Tracking V1 Webhook Ingestion (Deterministic Conventions)

## Scope Guard
This design covers only `provider signal webhook` ingestion for delivery tracking V1: boundary validation, normalization, persistence/handoff behavior, and deterministic concurrency/error handling for ingestion. It does not design `provider signal poll`, delivery-status progression, delivery-freshness progression, delivery read API behavior, observability naming, or implementation code.

## Inputs (Normative Inheritance Set)
- `specs/delivery-tracking/delivery-tracking-v1-webhook-ingestion.md`
- `specs/delivery-tracking/delivery-tracking-v1-service-topology.md`
- `specs/delivery-tracking/delivery-tracking-v1.md`
- `specs/delivery-tracking/overview.md`
- `specs/delivery-tracking/intent-correlation.md`
- `specs/delivery-tracking/ubiquitous-language.md`

## 1. Decision Statement
Choose one deterministic webhook-ingestion convention set so accepted `provider signal webhook` payloads normalize and hand off to correlation consistently under duplicates, out-of-order arrival, malformed optional timestamps, and multi-instance processing.

## 2. Options (1-3)

### Option A: Direct validate-and-handoff with no ingestion persistence
Short description:
- Validate and normalize request payload in memory, then immediately call correlation handoff.
- No persisted normalized ingestion record.

Pros:
- Lowest implementation complexity and lowest write overhead.
- Minimal storage footprint.

Cons:
- No durable ingestion trace at boundary.
- If handoff fails after partial in-process work, recovery depends entirely on provider resend.

Risks / failure modes:
- Harder operational diagnosis for dropped/transient failures.
- Less deterministic replay handling because no persisted boundary record exists.

Operational impact:
- Simple runtime path, no ingestion data retention to manage.

Compatibility with existing repo patterns:
- Medium. Simple, but weaker than current SQL-first durability conventions used elsewhere.

### Option B: Transactional normalize-and-handoff with persisted normalized ingestion record (Recommended)
Short description:
- Validate and normalize payload.
- In one SQL transaction, set `receivedAt` from database time, persist one normalized ingestion record, perform correlation handoff, and commit only if both persistence and handoff succeed.
- No duplicate suppression in ingestion; each valid payload is accepted and handed off deterministically.

Pros:
- Clear atomicity boundary: no partial persistence/handoff on failure.
- Deterministic ingestion record for accepted payloads.
- Keeps slice functional and synchronous without introducing async queue mechanics.

Cons:
- Duplicate/replayed valid payloads create additional accepted ingestion records.
- Requires strict transaction boundary discipline.

Risks / failure modes:
- If commit succeeds but webhook response is lost, provider retry can produce duplicate accepted records.
- Higher write load under duplicate/replay traffic.

Operational impact:
- One transaction per accepted payload.
- Predictable failure semantics and easier support/debug than Option A.

Compatibility with existing repo patterns:
- High. Aligns with SQL transaction-first behavior in current backend components.

### Option C: Persist normalized ingestion and use asynchronous queue-style handoff worker
Short description:
- Persist normalized records at webhook boundary, then hand off to correlation asynchronously via worker processing.

Pros:
- Better recovery from transient handoff outages.
- Decouples request latency from downstream availability.

Cons:
- Adds queue/worker lifecycle complexity.
- Requires retry policy, dead-letter behavior, and additional operational controls.

Risks / failure modes:
- Queue lag/backlog can delay downstream processing.
- More moving parts to debug in V1.

Operational impact:
- Additional runtime subsystem and monitoring requirements.

Compatibility with existing repo patterns:
- Medium. Feasible, but larger scope than needed for this V1 slice.

## 3. Recommendation (Exactly One)
Recommend **Option B**.

Rationale and tradeoffs:
- It provides deterministic atomic behavior for persistence plus handoff, without expanding V1 into queue/worker architecture.
- It preserves trusted-ingress functional scope and keeps behavior explicit under duplicates/replays.
- Tradeoff accepted: duplicates are not suppressed in ingestion and must remain benign downstream.

## 4. Approval Prompt (Exact Text)
Approve recommendation (Option B) for webhook-ingestion deterministic conventions.

## 5. Decision Record (Final Text)
Chosen option:
- Option B: transactional normalize-and-handoff with persisted normalized ingestion record.

### 5.0 Runtime ownership boundary
- `provider signal webhook` ingress is served only by delivery-tracking runtime instances behind HAProxy.
- SubmissionManager must not dual-serve or compatibility-proxy this ingress surface.
- Correctness must hold regardless of which healthy delivery-tracking runtime instance receives a webhook request.

### 5.1 Webhook normalization contract
- Accepted normalized handoff payload must include:
  - `intentId` (required)
  - `providerDeliverySignal` (required)
  - `providerObservedAt` (optional)
  - `receivedAt` (required, Setu ingestion time)
  - source metadata identifying `provider signal webhook`
- Normalization is required before correlation. Correlation receives only normalized fields, not provider-specific raw payload shape.

### 5.2 Validation and malformed timestamp handling
- Payload is invalid if required normalized fields are missing or structurally invalid (`intentId`, `providerDeliverySignal`).
- `providerObservedAt` is optional:
  - if absent: accept payload.
  - if present and valid timestamp: normalize to UTC and include.
  - if present but malformed/unparseable: do not reject payload; omit `providerObservedAt` and continue.
- `receivedAt` is always generated by Setu (database UTC time) for accepted payloads.

### 5.3 Idempotency and duplicate/replay handling
- Ingestion does not suppress duplicate/replayed valid payloads.
- Each valid payload instance is normalized and handed off once for that request path.
- Duplicate/replay safety is guaranteed by deterministic downstream handling (correlation/core semantics), not by webhook-ingestion suppression.
- Trusted-ingress-only in this round means replay protection controls are intentionally not part of this slice.

### 5.4 Timestamp precedence (`providerObservedAt` vs `receivedAt`)
- Deterministic precedence for downstream effective time:
  - Use `providerObservedAt` when present and valid.
  - Otherwise use `receivedAt`.
- Both values (when available) must be passed forward so downstream tie-break rules can be deterministic.

### 5.5 Persistence + handoff atomicity/failure behavior
- Atomic unit for accepted payload:
  1. Begin transaction.
  2. Generate `receivedAt` from database UTC time.
  3. Persist normalized ingestion record.
  4. Execute correlation handoff.
  5. Commit.
- Success response is returned only after commit.
- If persistence or handoff fails before commit, transaction is rolled back and no partial handoff is allowed.

### 5.6 Multi-instance concurrency and ordering behavior
- Any instance may process any `provider signal webhook` request.
- Ingestion does not guarantee arrival-order processing for the same `intentId`.
- Determinism requirements:
  - all instances use the same normalization rules.
  - all accepted payloads get `receivedAt` from database UTC time (not host local time).
  - downstream must remain order-independent for duplicates and out-of-order signals, consistent with foundational contracts.

### 5.7 Error mapping (invalid payload vs transient internal failure)
- Invalid payload (required normalized fields invalid/missing): reject as client/input error (`400` class).
- Transient internal failure (database unavailable, transaction deadlock/timeout, handoff unavailable): fail request as transient server failure (`503` class).
- Malformed `providerObservedAt` alone is not an invalid-payload failure; fallback behavior applies.

## Risks and Failure Modes (Chosen Option)
- Duplicate/replayed valid payloads can increase ingestion and downstream load.
- If commit succeeds but response does not reach provider, provider retry can produce duplicate accepted records.
- High duplicate traffic may increase storage and processing costs without changing final delivery semantics.
- Database-time dependency means database availability directly affects ingestion acceptance.

## Explicit Deferrals
- Webhook authentication is deferred.
- Signature verification is deferred.
- Replay protection controls are deferred.
- `provider signal poll` is deferred.
- Delivery read API contracts are deferred.
- Observability metric/log naming is deferred.
- Async queue-style webhook handoff worker is deferred in this V1 slice.

## ADR (Concise)
- ADR-INGEST-001: Use transactional normalize-and-handoff for accepted `provider signal webhook` payloads.
- ADR-INGEST-002: Treat malformed `providerObservedAt` as fallback-to-`receivedAt`, not hard reject.
- ADR-INGEST-003: Keep duplicate/replay suppression out of webhook-ingestion in V1; require deterministic downstream convergence.
