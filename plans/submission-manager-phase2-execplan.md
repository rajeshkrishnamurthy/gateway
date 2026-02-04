# Phase 2: SubmissionIntent execution engine (core, in-memory)

This execplan is a living document. The sections Progress, Surprises & Discoveries, Decision Log, and Outcomes & Retrospective must be kept up to date as work proceeds.

This plan follows `backend/PLANS.md` from the repository root and must be maintained in accordance with it.

## Purpose / Big Picture

After this change, SubmissionManager has a core, HTTP-agnostic execution engine that accepts SubmissionIntents, performs the first attempt immediately, and completes each intent as ACCEPTED, REJECTED, or EXHAUSTED based on gateway outcomes and the selected contract policy. SubmissionManager remains the sole orchestrator: it resolves submissionTarget to a contract snapshot and routing (gatewayType + gatewayUrl), and it passes that resolved routing into the executor rather than letting execution re-resolve or infer it. Statuses are orchestration decisions, not gateway statuses; gateway outcomes are stored as facts and then evaluated against the contract policy. Retry timing uses a fixed 5 second delay in Phase 2 as an internal execution policy, not a contract term, and is designed to be extendable later. This phase stays in-memory only and relies on the Phase 1 SubmissionTarget registry for explicit gatewayType binding and contract rules. Success is observable via unit tests that exercise policy-based exhaustion, idempotency conflicts, terminal outcomes, and fixed-delay retry scheduling without running any HTTP servers.

## Progress

- [x] (2026-01-27 21:30Z) Review Phase 1 registry outputs, gateway outcomes, and execplan requirements for the Phase 2 engine.
- [x] (2026-01-28 10:55Z) Implement the in-memory intent store and attempt history model, including a resolved contract snapshot.
- [x] (2026-01-28 11:05Z) Implement the SubmissionManager execution loop with a single worker and deterministic scheduling.
- [x] (2026-01-28 11:25Z) Add contract-aware attempt evaluation (terminal outcomes and policy-specific completion rules) with explicit gateway-outcome vs policy-decision separation and clear intent-status semantics.
- [x] (2026-01-28 12:05Z) Add tests that cover idempotency conflicts, terminal outcomes, policy-based exhaustion, and routing/orchestration boundaries.
- [x] (2026-01-28 12:15Z) Update minimal docs to point at the Phase 2 engine and its limits.

## Surprises & Discoveries

- Observation: `backend/submission/AGENTS.md` forbids scheduling or execution in that package, so the Phase 2 engine must live elsewhere.
  Evidence: `backend/submission/AGENTS.md` scope and boundaries section.

## Decision Log

- Decision: Keep Phase 2 state in-memory only and avoid persistence or migrations.
  Rationale: Phase 2 is focused on core execution semantics; durability is a later decision.
  Date/Author: 2026-01-27 / Codex

- Decision: Use a single worker loop with a scheduled-at queue to run attempts in order; do not spawn per-intent goroutines.
  Rationale: Minimizes concurrency, keeps lifetimes explicit, and avoids fire-and-forget work.
  Date/Author: 2026-01-27 / Codex

- Decision: Model payload as json.RawMessage (opaque bytes) and compare raw bytes for idempotency conflicts.
  Rationale: Keeps the payload opaque while enabling strict idempotency checks without decoding.
  Date/Author: 2026-01-27 / Codex

- Decision: Treat accepted as always terminal; treat terminalOutcomes as contract-defined terminal reasons; apply policy-specific exhaustion rules for non-terminal rejections.
  Rationale: Matches the contract semantics and keeps SubmissionManager from reinterpreting gateway behavior.
  Date/Author: 2026-01-27 / Codex

- Decision: Enforce policy-specific termination rules:
  - `deadline`: cumulative wall-clock deadline from intent creation.
  - `max_attempts`: attempt count limit.
  - `one_shot`: single attempt, no retries.
  Rationale: Makes contract termination explicit and avoids implicit coupling to a single rule.
  Date/Author: 2026-01-27 / Codex

- Decision: Use an explicit idempotency conflict error when the same intentId is submitted with a different submissionTarget or payload.
  Rationale: Prevents silent drift and ensures deterministic intent ownership.
  Date/Author: 2026-01-27 / Codex

- Decision: Include routing in executor inputs (resolved gatewayUrl + gatewayType) and store a contract snapshot on the intent at submission time.
  Rationale: Keeps routing responsibility explicit in SubmissionManager and avoids implicit resolution in execution.
  Date/Author: 2026-01-27 / Codex

- Decision: Separate gateway outcomes (facts) from intent completion decisions (policy).
  Rationale: Prevents semantic drift and keeps policy evaluation explicit.
  Date/Author: 2026-01-27 / Codex

- Decision: Treat intent statuses as orchestration outcomes derived from policy (accepted/rejected/exhausted), never as gateway response aliases.
  Rationale: Keeps long-lived intent state unambiguous even as gateway outcome taxonomies evolve.
  Date/Author: 2026-01-28 / Codex

- Decision: Keep idempotency conflicts diagnosable by returning a typed error that includes the conflicting fields and the existing intent’s status.
  Rationale: Ensures callers and operators can distinguish idempotency conflicts from transient failures.
  Date/Author: 2026-01-28 / Codex

- Decision: Use a fixed 5 second retry delay in Phase 2 as an internal execution policy, not as part of the contract; keep the design open to future policies.
  Rationale: Enables deterministic tests now without exposing timing as a client-visible contract commitment.
  Date/Author: 2026-01-28 / Codex

- Decision: Implement the Phase 2 engine in a new `backend/submissionmanager` package rather than `backend/submission`.
  Rationale: `backend/submission` is reserved for registry/contract validation and explicitly forbids scheduling or attempt execution.
  Date/Author: 2026-01-28 / Codex

## Outcomes & Retrospective

Phase 2 now has an in-memory SubmissionManager engine (`backend/submissionmanager`) with intent storage, scheduling, policy evaluation, and deterministic tests. The retry delay is fixed at 5 seconds as an internal policy, with explicit extension points for future policies. Documentation now distinguishes the registry package from the execution engine. Remaining gaps are persistence, HTTP APIs, and configurable retry policies, which are intentionally deferred.

## Context and Orientation

Phase 1 introduced the SubmissionTarget registry and contract validation in `backend/submission/registry.go` with a sample registry config at `backend/conf/submission/submission_targets.json` and the formal spec in `specs/submission-manager.md`. The gateways (SMS and push) remain synchronous and submission-only; their response reasons are the canonical outcome taxonomy. SubmissionManager must use gatewayType (code-known) to interpret gateway outcomes, while submissionTarget selects the contract (data-driven). SubmissionManager is the only component allowed to resolve submissionTarget into a contract snapshot and routing; the executor only consumes that resolved routing. Intent status is an orchestration outcome, not a gateway outcome. This phase adds the core in-memory execution engine under a new `backend/submissionmanager/` package (separate from the registry package to respect submission package boundaries), keeping it HTTP-agnostic and dependency-free.

## Plan of Work

Add a core SubmissionManager implementation in the `backend/submissionmanager` package that owns intent storage, attempt scheduling, and contract-aware completion. The manager should load contracts via the Phase 1 registry (`backend/submission`) and accept SubmissionIntents with opaque payloads. On submission, it should resolve and store a contract snapshot (including gatewayType and gatewayUrl) on the intent, and pass the resolved routing to the executor; execution must not re-resolve or infer routing. It should run a single worker loop that executes attempts in order of due time. Completion should be policy-aware: deadline-based exhaustion, max-attempt exhaustion, or one-shot completion. Retry timing uses a fixed 5 second delay constant in Phase 2 as an internal execution policy (not a contract term), but is deliberately structured to allow future policies. Intent statuses must remain orchestration decisions derived from gateway outcomes plus policy evaluation (accepted, rejected, exhausted), not aliases for gateway responses. The manager should expose query methods that return intent status, gateway outcomes, and attempt history. Tests should inject a controllable clock and stub executors to validate terminal outcomes, idempotency conflicts, policy-specific exhaustion, routing/orchestration boundaries, and fixed-delay retry scheduling without real time sleeps.

## Concrete Steps

1) Add intent, attempt, and status types in `backend/submissionmanager/manager.go` (or new files in the same package) with explicit fields for createdAt, status, resolved contract snapshot, attempts, and last gateway outcome. Document in code comments that FinalOutcome is meaningful only for ACCEPTED/REJECTED and that ExhaustedReason explains policy exhaustion rather than gateway failure.

2) Implement an in-memory store guarded by a mutex that indexes intents by intentId and preserves attempt history in order.

3) Implement the SubmissionManager worker loop with a scheduled-at queue, using a single goroutine started by `Run(ctx)` and stopped when the context is canceled. Use a small clock struct (Now/After funcs) to allow deterministic tests. Execution must pass resolved routing (gatewayType + gatewayUrl) to the executor, and no execution path should resolve routing on its own.

4) Implement contract-aware evaluation with explicit fact vs policy separation: capture the gateway outcome as a fact, then apply policy to decide completion. Intent status is derived from policy evaluation, not from the gateway outcome names. Accepted is terminal; if rejected with a terminal outcome, mark REJECTED; if rejected with a non-terminal outcome, apply the policy:
   - `deadline`: retry while the acceptance deadline has not passed.
   - `max_attempts`: retry while attempt count is below maxAttempts.
   - `one_shot`: no retries; mark EXHAUSTED on non-terminal outcomes.

5) Add tests under `backend/submissionmanager/manager_test.go` that cover idempotency conflicts, terminal outcomes, policy-based exhaustion, routing/orchestration boundaries, fixed-delay retry scheduling, and “unknown gateway response” errors. Ensure tests do not rely on real time sleeps. Include explicit cases for one_shot, max_attempts, deadline, duplicate_reference non-terminal handling, contract snapshot stability after registry changes, idempotency conflict error contents (conflicting fields + existing status), and a fixed 5 second retry schedule assertion.

6) Add `backend/submissionmanager/README.md` (and optionally `AGENTS.md`) to describe the new package’s role and boundaries. Update `backend/README.md` with a short Phase 2 pointer if needed.

## Validation and Acceptance

From `backend/`, run:

  go test ./...

Acceptance is met when the Phase 2 tests demonstrate:

- same intentId + same payload returns the existing intent without duplication
- same intentId + different payload or target returns an idempotency conflict error
- a terminal rejection ends the intent as REJECTED without retries
- a retryable rejection schedules another attempt and eventually ACCEPTED when the stub executor returns accepted
- a deadline policy intent that completes after the acceptance deadline results in EXHAUSTED
- a max_attempts policy intent exhausts after the configured attempt count
- a one_shot policy intent exhausts after a non-terminal rejection
- a duplicate_reference outcome is treated as non-terminal and does not complete the intent
- a contract snapshot remains unchanged even if the registry is modified after submission
- a routing change in the registry after submission does not affect the stored contract snapshot or executor inputs
- an idempotency conflict error includes the conflicting target/payload and the existing intent status for diagnosis
- retry scheduling uses the fixed 5 second delay constant in Phase 2 (and this delay is not surfaced as contract semantics)

## Idempotence and Recovery

The manager is in-memory only. Restarting clears state. Tests and the manager’s API must handle repeated submissions safely: the same intentId returns the same record unless there is a conflict. Idempotency conflicts must be explicit and diagnosable via a typed error, not via silent overwrites. The worker loop should exit cleanly on context cancellation without leaving pending goroutines.

## Artifacts and Notes

Retry timing uses a fixed 5 second delay constant in Phase 2, treated as an internal execution policy. It must not be surfaced as a contract term. Keep the scheduling logic small and deterministic to support tests, and retain an obvious extension point for richer policies in a later phase.

## Interfaces and Dependencies

Use only the Go standard library and the existing `gateway/submission` package. Add the following types and functions under `backend/submissionmanager/`:

  type IntentStatus string

  const (
    IntentPending   IntentStatus = "pending"
    IntentAccepted  IntentStatus = "accepted"
    IntentRejected  IntentStatus = "rejected"
    IntentExhausted IntentStatus = "exhausted"
  )

  type Intent struct {
    IntentID         string
    SubmissionTarget string
    Payload          json.RawMessage
    CreatedAt        time.Time
    Status           IntentStatus
    Contract         TargetContract
    Attempts         []Attempt
    FinalOutcome     GatewayOutcome // meaningful for accepted/rejected intents only
    ExhaustedReason  string         // explains policy exhaustion, not gateway failure
  }

  type Attempt struct {
    Number         int
    StartedAt      time.Time
    FinishedAt     time.Time
    GatewayOutcome GatewayOutcome
    Error          string
  }

  type GatewayOutcome struct {
    Status string
    Reason string
  }

  type AttemptInput struct {
    GatewayType GatewayType
    GatewayURL  string
    Payload     json.RawMessage
  }

  type AttemptExecutor func(context.Context, AttemptInput) (GatewayOutcome, error)

  type Clock struct {
    Now   func() time.Time
    After func(time.Duration) <-chan time.Time
  }

  type Manager struct {
    // holds registry, store, executor, clock, queue, and state
  }

  func NewManager(reg Registry, exec AttemptExecutor, clock Clock) (*Manager, error)
  func (m *Manager) Run(ctx context.Context)
  func (m *Manager) SubmitIntent(ctx context.Context, intent Intent) (Intent, error)
  func (m *Manager) GetIntent(intentID string) (Intent, bool)

  type IdempotencyConflictError struct {
    IntentID         string
    ExistingTarget   string
    ExistingPayload  string
    IncomingTarget   string
    IncomingPayload  string
    ExistingStatus   IntentStatus
  }

  func (e IdempotencyConflictError) Error() string

The executor is a concrete function type rather than an interface to avoid extra abstraction. Contract lookups must use the Phase 1 registry and refuse unknown submissionTarget values.

---

Change log: Phase 2 plan drafted to implement the core in-memory SubmissionManager execution engine. (2026-01-27 / Codex)
Change log: Updated Phase 2 plan to use explicit contract policy semantics (deadline, max_attempts, one_shot). (2026-01-27 / Codex)
Change log: Clarified routing vs execution ownership, intent status semantics, explicit idempotency conflict signaling, gateway outcome vs contract policy separation, and added test expectations; deferred retry timing specifics. (2026-01-28 / Codex)
Change log: Set Phase 2 retry timing to a fixed internal delay (non-contractual) while keeping future policy extension open. (2026-01-28 / Codex)
Change log: Chose a concrete fixed delay of 5 seconds to make execution and tests explicit. (2026-01-28 / Codex)
Change log: Clarified FinalOutcome vs ExhaustedReason semantics in the Intent struct. (2026-01-28 / Codex)
Change log: Moved the Phase 2 engine to a new `backend/submissionmanager` package to respect registry package boundaries. (2026-01-28 / Codex)
Change log: Marked Phase 2 implementation steps complete and recorded the outcome summary. (2026-01-28 / Codex)
