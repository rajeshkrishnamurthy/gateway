# Design: Delivery Tracking V1 Core Processing (Deterministic Conventions)

## Scope Guard
This design is limited to the delivery core processing slice from correlated signal input through delivery status/freshness/history persistence. It does not design webhook auth, provider-specific payload contracts, delivery read endpoint payloads, or observability naming beyond fields needed for deterministic core writes.

## Inputs
- `specs/delivery-tracking/delivery-tracking-v1.md`
- `specs/delivery-tracking/delivery-tracking-v1-core-processing.md`
- `specs/delivery-tracking/delivery-tracking-v1-service-topology.md`
- `designs/delivery-tracking-v1-webhook-ingestion.md`
- Foundational delivery-tracking specs:
  - `specs/delivery-tracking/overview.md`
  - `specs/delivery-tracking/delivery-status-model.md`
  - `specs/delivery-tracking/delivery-freshness.md`
  - `specs/delivery-tracking/intent-correlation.md`
  - `specs/delivery-tracking/ubiquitous-language.md`

## 1. Decision Statement
Choose one deterministic core-processing convention set for applying correlated provider delivery signals so status/freshness/history outcomes are atomic, replay-safe, and convergent under duplicate, out-of-order, and concurrent processing.

## 2. Options (1-3)

### Option A: Project-on-write inside per-intent SQL transaction (history + current state), with terminal lock
Short description:
- On each matched signal, write one history row and update one current-state row atomically in the same transaction.
- Use a per-intent transaction lock and a terminal lock rule (`first terminal polarity wins for current status`).
- Run staleness (`fresh -> stale`) via a small evaluator job; run `stale -> fresh` only in signal-apply path.

Pros:
- Strong atomicity and low read complexity (`GET /delivery` reads one current-state row).
- Aligns with existing repository persistence style (SQL transaction boundaries, deterministic row updates).
- Straightforward idempotency via unique source record key.

Cons:
- Requires one projection table plus history table and lock ordering discipline.
- Needs a background staleness evaluator to avoid read-time recomputation.

Risks / failure modes:
- If evaluator is down, stale transitions are delayed (state remains last known, which is spec-compliant).
- Incorrect lock ordering can introduce deadlocks if not standardized.

Operational impact:
- One short transaction per matched signal.
- One periodic SQL update loop for stale classification.
- Simple debugging with durable history + current projection.

Compatibility with existing repo patterns:
- High. Matches SubmissionManager’s SQL-first determinism and transactional update style.

### Option B: Append-only history only; derive current status/freshness on every read
Short description:
- Persist only accepted correlated signal history.
- Compute current delivery status/freshness dynamically during read endpoints.

Pros:
- Simple write path.
- Minimal projection schema.

Cons:
- Read path becomes complex and expensive.
- Freshness depends on read-time clock, increasing endpoint-time variability.

Risks / failure modes:
- High read amplification and harder SLO control.
- Harder incident triage because current state is not materialized durably.

Operational impact:
- Lower write cost, higher read CPU/IO cost.
- More expensive backfills and API hot paths.

Compatibility with existing repo patterns:
- Medium-low. Current system favors explicit persisted current state for deterministic reads.

### Option C: Append history synchronously, project current state asynchronously
Short description:
- Ingestion/core write only history; separate worker projects current state later.

Pros:
- Write latency can remain low.
- Projection logic isolated.

Cons:
- Eventual consistency between history and current status/freshness.
- More moving parts (queue/offset/replayer semantics).

Risks / failure modes:
- Projection lag or stuck projector causes stale/incorrect current view until catch-up.
- Replay and exactly-once complexity is higher than needed for V1.

Operational impact:
- Requires projector lifecycle monitoring and replay tooling.
- More failure surfaces for V1.

Compatibility with existing repo patterns:
- Medium. Repository currently prefers direct transactional state mutation over async projection for core semantics.

## 3. Recommendation (Exactly One)
Recommend **Option A**.

Rationale and tradeoffs:
- It is the smallest deterministic model that satisfies atomic core-application semantics, append-meaning history, and predictable current-state reads.
- It avoids eventual-consistency complexity (Option C) and avoids heavy read-time recomputation (Option B).
- Tradeoff accepted: add one focused staleness evaluator loop and strict lock ordering.

## 4. Approval Prompt (Exact Text)
Approve recommendation (Option A) for delivery core processing conventions.

## 5. Decision Record (Final Text)
Chosen option:
- Option A: transactional project-on-write with append history and terminal lock.

Reliability boundary note:
- Durable queueing/inbox-worker decoupling is intentionally out of V1 scope for this slice.
- Context and rationale are recorded in `designs/adr/0001-delivery-tracking-durable-queueing.md`.
- This design therefore assumes direct synchronous invocation from the ingress request path after ingress-audit persistence commits, with transactional apply for history/state and idempotent no-op handling on replay/duplicate apply attempts.
- No persisted correlation-handoff queue/inbox stage is required for correctness in V1.

Convention and mapping definition:

1) Canonical transition application flow
- Core input is a normalized correlated record with:
  - `sourceRecordId` (stable ingestion record id; required, generated by Setu at ingress-audit persistence boundary)
  - `providerEventId` (optional provider-supplied event id, if available; not required by Setu contract)
  - `intentId` (required)
  - `providerDeliverySignal` (normalized to one of: `in_progress`, `terminal_success`, `terminal_failure`)
  - Note: `unknown` is not a provider signal class; it is the initial current `deliveryStatus` when no authoritative signal has been observed.
  - `providerObservedAt` (optional)
  - `receivedAt` (required, UTC)
  - `correlationResult` (`matched`/`unmatched`/`invalid`)
- Apply flow:
  0. Input arrives from synchronous ingress-triggered invocation (no asynchronous queue/inbox dependency in V1).
  1. If `correlationResult` is `unmatched` or `invalid`, stop with no domain mutation.
  2. Start SQL transaction.
  3. Lock parent intent row by `intentId` (row-level update lock) and read snapshotted delivery-tracking contract inputs (`mode`, `staleAfterSeconds`) plus submission terminal fields (`submission status`, `completedAt`).
  4. If `mode=off`, commit no-op for domain state (observability/audit handled outside this slice).
  5. Insert history row idempotently by `sourceRecordId`.
  6. If insert was duplicate (`sourceRecordId` already present), commit and return idempotent no-op.
  7. Compute ordering key `K` (defined below) and compute `lateObservation` for terminal signals.
  8. Load and lock current delivery projection row for this intent (or create it if absent).
  9. Apply deterministic transition rules (below) and update projection row.
  10. Commit transaction.

2) Ordering and idempotency rules
- Effective time precedence:
  - `effectiveAt = providerObservedAt` when present/valid.
  - Else `effectiveAt = receivedAt`.
- Canonical ordering key:
  - `K = (effectiveAt, receivedAt, sourceRecordId)` with strict lexicographic compare.
- Equal effective-time tie-break rule:
  - First tie-breaker: `receivedAt`.
  - Second tie-breaker: `sourceRecordId` lexical order.
- Idempotency:
  - Hard idempotency key: `(intentId, sourceRecordId)` unique in history.
  - Re-processing the same `sourceRecordId` is a no-op.
  - This idempotency is for replay/retry of the same accepted Setu ingress record; it does not require providers to supply a unique event id.
- Deterministic meaning of `newer valid signal`:
  - A signal is `newer valid` for stale-to-fresh reclassification iff:
    - correlation is `matched`
    - mode is `on`
    - normalized signal class is `in_progress`
    - `K > lastNonTerminalKey` stored in current-state projection.

3) Deterministic transition rules (status + freshness)
- Initial tracked projection when absent:
  - `deliveryStatus=unknown`
  - `deliveryFreshness=fresh`
  - `lastNonTerminalKey=nil`
  - `terminalLocked=false`
  - `staleAt = intent.createdAt + staleAfterSeconds`
- For `in_progress` signal:
  - If `terminalLocked=true`, keep current status/freshness; append history only.
  - If `K > lastNonTerminalKey`, set:
    - `deliveryStatus=in_progress` when current is `unknown` or `in_progress`
    - `deliveryFreshness=fresh`
    - `lastNonTerminalKey=K`
    - `staleAt = effectiveAt + staleAfterSeconds`
  - Else (duplicate/non-newer), no status/freshness change.
- For terminal success signal:
  - If `terminalLocked=false`, set `deliveryStatus=delivered`, `deliveryFreshness=not_applicable`, lock terminal fields (`terminalLocked=true`, `terminalStatus=delivered`, `terminalKey=K`), clear `staleAt`.
  - If `terminalLocked=true`, append history only.
- For terminal failure signal:
  - If `terminalLocked=false`, set `deliveryStatus=failed`, `deliveryFreshness=not_applicable`, lock terminal fields (`terminalLocked=true`, `terminalStatus=failed`, `terminalKey=K`), clear `staleAt`.
  - If `terminalLocked=true`, append history only.

4) Late terminal signal handling
- `lateObservation=true` for a terminal signal iff submission was already terminal by the time Setu received that signal:
  - `submission status in {accepted,rejected,exhausted}`
  - and `completedAt <= receivedAt`.
- Late terminal signals always remain visible in history.
- Submission fields are never mutated by delivery core processing.
- Terminal polarity conflicts:
  - Current delivery status is immutable after first terminal lock.
  - Opposite-polarity terminal signals are recorded in history with conflict marker fields (for diagnosis), but do not change current delivery status.

5) Stale/fresh evaluation placement
- Signal-apply path performs:
  - `stale -> fresh` only on `newer valid in_progress` signal (rule above).
  - terminal signal transition to `not_applicable`.
- Time-driven `fresh -> stale` occurs in a periodic staleness evaluator job:
  - Update condition: `deliveryStatus in {unknown,in_progress}` and `deliveryFreshness=fresh` and `staleAt <= SYSUTCDATETIME()`.
  - Update action: set `deliveryFreshness=stale`.
- Read path does not recompute freshness ad hoc; it reads persisted freshness/projection state.

6) Persistence write model and transaction boundaries
- Tables for this slice:
  - `dbo.intent_delivery_state` (1 row per tracked intent; current projection + ordering watermarks).
  - `dbo.intent_delivery_history` (append-only semantic history; one row per accepted correlated signal record).
- Required keys/indexes:
  - `intent_delivery_state`: PK `(intent_id)`.
  - `intent_delivery_history`: clustered PK `(intent_id, history_seq)` or equivalent append key.
  - `intent_delivery_history`: unique key `(intent_id, source_record_id)` for idempotency.
  - `intent_delivery_state`: index on `(delivery_status, delivery_freshness, stale_at)` for evaluator.
- Transaction boundary:
  - One transaction wraps intent lock, idempotent history insert, projection read/update, and history annotation fields (`lateObservation`, conflict marker).
  - No partial commit is allowed between history insert and projection update.

7) Concurrency model and conflict handling
- Concurrency control is database-first, not process-local:
  - Serialize per-intent signal application using row-level update lock on parent intent row (and projection row in same transaction).
  - Lock ordering must be fixed: `submission_intents` row -> `intent_delivery_state` row -> history insert/update.
- Multi-instance safety:
  - Any instance may process a signal; convergence is enforced by transactional locks + idempotency key.
- Conflict classes:
  - Duplicate delivery record (`sourceRecordId`): no-op.
  - Non-newer non-terminal: history-only append, no reclassification.
  - Opposite terminal after terminal lock: history conflict marker only, no current-state change.
  - Evaluator vs signal race: serialized by row lock; whichever commits last still respects transition guards.

Configuration parameters:
- `staleAfterSeconds` (snapshotted per intent, from contract, required when mode is `on`).
- Evaluator tick interval (internal runtime knob; not client contract).
- No public per-target knob is added for conflict strategy or stale-to-fresh behavior.

Constraints that must hold:
- Delivery core never mutates submission status or submission completion timestamps.
- `mode=off` never mutates delivery status/freshness.
- `unmatched`/`invalid` never mutate delivery status/freshness.
- Current delivery status and freshness remain derivable from persisted projection + history under the above rules.
- Correctness must hold under multi-instance delivery-tracking runtime execution behind HAProxy without sticky-session dependence.
- Correctness does not depend on any persisted correlation-handoff queue/inbox artifact in V1.

## ADR (Concise)
- ADR-001: Use transactional project-on-write with one current-state row plus append history row per accepted correlated signal.
- ADR-002: Use ordering key `K=(effectiveAt,receivedAt,sourceRecordId)`; `effectiveAt` precedence is `providerObservedAt` then `receivedAt`.
- ADR-003: Define `newer valid signal` as `matched + mode on + in_progress + K > lastNonTerminalKey`.
- ADR-004: Terminal conflict policy is terminal lock (`first terminal polarity wins current status`), with opposite polarity recorded as history conflict only.
- ADR-005: Place `fresh->stale` in periodic evaluator; place `stale->fresh` only in signal-apply path on newer valid `in_progress` signals.

## Spec Wording Clarifications Needed (No Spec Edits Applied Here)
- `specs/delivery-tracking/delivery-status-model.md`
  - Clarify terminal conflict handling explicitly: current status locks on first terminal polarity; opposite terminal polarity becomes history-only conflict annotation.
- `specs/delivery-tracking/delivery-freshness.md`
  - Clarify canonical ordering key and exact `newer valid signal` compare rule.
  - Clarify that `fresh->stale` is persisted by evaluator path, while `stale->fresh` is signal-triggered only.
- `specs/delivery-tracking/delivery-tracking-v1-core-processing.md`
  - Clarify transaction boundary includes idempotent history write + current-state mutation in one atomic unit.
