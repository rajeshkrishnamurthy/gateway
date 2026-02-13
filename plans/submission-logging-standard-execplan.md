# Submission bounded context structured logging standard rollout

This execplan is a living document. The sections Progress, Surprises & Discoveries, Decision Log, and Outcomes & Retrospective must be kept up to date as work proceeds.

This plan follows `backend/PLANS.md` from the repository root and must be maintained in accordance with it.

EXECPLAN-READY

## Purpose / Big Picture

After this change, submission bounded context runtime evidence (gateway decisions, provider mapping decisions, SubmissionManager attempt lifecycle, and lease leadership transitions) is emitted as machine-parseable structured logs using `log/slog`, aligned with `specs/logging-standard.md`. Operators can query stable fields (`event`, `commProfile`, `intentId`, `referenceId`, `outcome`, etc.) instead of parsing free-form text. A repository-owned parser validates log schema/profile requirements and supports current plus previous schema versions, enabling proactive checks and safer troubleshooting tooling.

This rollout is intentionally limited to submission bounded context. Delivery-tracking logging is explicitly out of scope for this execplan.

## Progress

- [x] (2026-02-13 00:00Z) Confirm scope with user: submission bounded context only.
- [x] (2026-02-13 00:00Z) Read required mode and component instructions (`AGENTS.md`, `agents/EXEC.md`, `backend/AGENTS.md`, `backend/cmd/*/AGENTS.md`, `backend/adapter/AGENTS.md`, `backend/submissionmanager/AGENTS.md`) plus relevant specs.
- [x] (2026-02-13 10:00Z) Create shared logging package (`backend/logging`) for schema/profile constants and `slog` bootstrap; bootstrap wired in submission service mains.
- [x] (2026-02-13 10:00Z) Migrate submission decision/evidence logs in SMS/Push gateways and all submission provider adapters to structured `slog`.
- [x] (2026-02-13 10:00Z) Migrate SubmissionManager attempt lifecycle and leader lease events to structured `slog` with required profile fields.
- [x] (2026-02-13 10:00Z) Add `setu_to_setu` logging evidence for SubmissionManager internal gateway HTTP calls in `backend/cmd/submission-manager/executor.go`.
- [x] (2026-02-13 10:00Z) Add parser/normalizer package and tests for schema/profile validation and redaction-sensitive fixtures.
- [x] (2026-02-13 10:00Z) Run focused and full backend tests; capture evidence and update plan sections.
- [x] (2026-02-13 18:19Z) Demote high-frequency `leader_renewed` lease-heartbeat event from `Info` to `Debug` in SubmissionManager leader runner to reduce operational log noise.
- [x] (2026-02-13 18:27Z) Demote repeated `leader_acquire_not_granted` leadership-attempt event from `Warn` to `Debug` in SubmissionManager leader runner to reduce normal multi-instance churn noise.

## Surprises & Discoveries

- Observation: Existing submission logs are broad and mostly `log.Printf`, but they already include stable semantic keys (`referenceId`, `status`, `reason`, leader events), which lowers migration risk.
  Evidence: `rg -n "log\\.Printf"` across `backend/cmd/sms-gateway`, `backend/cmd/push-gateway`, `backend/adapter`, and `backend/submissionmanager`.

- Observation: Setu-to-Setu executor logging needed `intentId` for end-to-end attribution, but `submissionmanager.AttemptInput` did not previously carry it.
  Evidence: `backend/submissionmanager/manager.go` originally defined `AttemptInput` without `IntentID`; executor had no direct access to intent correlation key.

## Decision Log

- Decision: Scope implementation to submission bounded context only (gateways, adapters, SubmissionManager, and submission-manager executor).
  Rationale: User explicitly constrained rollout scope; this still satisfies a meaningful slice of `specs/logging-standard.md` while avoiding delivery-tracking churn.
  Date/Author: 2026-02-13 / Codex

- Decision: Keep one canonical structured logging stream and avoid duplicate plain-text decision logs.
  Rationale: Avoid schema drift and duplicate evidence paths; aligns with `specs/logging-standard.md`.
  Date/Author: 2026-02-13 / Codex

- Decision: Extend `submissionmanager.AttemptInput` with `IntentID` to preserve `setu_to_setu` correlation evidence in executor logs.
  Rationale: `specs/logging-standard.md` requires stable correlation keys on boundary operations; adding `IntentID` keeps attribution deterministic without changing runtime semantics.
  Date/Author: 2026-02-13 / Codex

- Decision: Emit `leader_renewed` as `Debug` instead of `Info`.
  Rationale: Lease renewals are high-frequency heartbeat evidence; keeping them at `Info` creates avoidable noise during normal steady-state operation, while `Debug` retains diagnosability when needed.
  Date/Author: 2026-02-13 / Codex

- Decision: Emit `leader acquire not granted` as `Debug` instead of `Warn`.
  Rationale: Competing leader instances frequently and legitimately log failed acquire attempts during normal operation; lowering this to `Debug` preserves observability while preventing false-alarm noise in normal readiness states.
  Date/Author: 2026-02-13 / Codex

## Outcomes & Retrospective

Submission bounded context now emits structured `slog` decision/evidence logs across all targeted surfaces:

- client-to-Setu gateway decisions (`outside_to_setu`, ingress),
- Setu-to-provider adapter call/mapping evidence (`outside_to_setu`, egress),
- SubmissionManager internal lifecycle and lease evidence (`within_setu_service`),
- SubmissionManager-to-gateway HTTP execution evidence (`setu_to_setu`).

A shared logging package now provides schema/profile constants and runtime logger bootstrap with base fields (`logSchemaVersion`, `service`, `instance`). A parser/normalizer with tests validates required schema/profile fields and catches malformed or redaction-sensitive invalid records.

`leader_renewed` remains emitted with full structured fields, but now at `Debug` severity to keep `Info` logs focused on meaningful state changes and failures.

`leader acquire not granted` now logs at `Debug` severity to avoid repeated `Warn` noise from normal non-leader instances while preserving the same structured evidence payload.

Prior full backend test runs passed for this rollout; in the current local run window, SQL-backed `submissionmanager` tests require an available SQL Server on `localhost:1433`, so compile-only fallback plus `logging` tests were used for this incremental severity change.

## Context and Orientation

Submission bounded context spans:

- Gateway HTTP boundaries: `backend/cmd/sms-gateway/handlers.go`, `backend/cmd/push-gateway/handlers.go`.
- Gateway core panic normalization: `backend/sms_gateway.go`, `backend/push_gateway.go`.
- Provider boundary adapters: `backend/adapter/*.go`.
- Submission execution lifecycle and leadership: `backend/submissionmanager/attempts.go`, `backend/submissionmanager/leader_runner.go`.
- Internal service-to-service gateway HTTP execution from SubmissionManager: `backend/cmd/submission-manager/executor.go`.

Bootstrap entrypoints for the above are:

- `backend/cmd/sms-gateway/main.go`
- `backend/cmd/push-gateway/main.go`
- `backend/cmd/submission-manager/main.go`

Relevant contracts:

- `specs/logging-standard.md` (primary)
- `specs/gateway-contracts.md` (gateway decision fields)
- `specs/model-provider-adapter.md` and `backend/adapter/AGENTS.md` (adapter logging safety + mapping discipline)
- `specs/submission-tracking/submission-manager-leaderlease.md` (required leader events/fields)

## Plan of Work

Introduce a small shared logging module under `backend/logging` that exposes:

- stable schema/profile constants (`logSchemaVersion`, `commProfile` values),
- `slog` bootstrap for service entrypoints with base fields (`logSchemaVersion`, `service`, `instance`),
- small helpers for common profile labels and bounded values.

Then replace submission decision/evidence `log.Printf` calls with structured `slog` calls that include:

- `event`,
- `commProfile`,
- profile-required fields (`boundaryDirection`, `peerSystem`, `operation`, `outcome`, and for inter-service calls `callerService`, `calleeService`, `durationMs`),
- existing domain correlation fields (`referenceId`, `intentId`, `attempt`, `gatewayMessageId`, `provider`, `leaseEpoch`, etc.).

Add parser/normalizer code under `backend/logging` that parses JSON lines, validates required/base/profile-specific fields, validates supported schema versions, and fails safely for malformed lines. Add tests for valid lines across the three `commProfile` values and negative fixtures for missing fields, malformed JSON, invalid profile values, and redaction-sensitive forbidden fields.

## Concrete Steps

1. Create `backend/logging/` package files for constants and `Configure(service string)` logger bootstrap.
2. Call logger bootstrap in:
   - `backend/cmd/sms-gateway/main.go`
   - `backend/cmd/push-gateway/main.go`
   - `backend/cmd/submission-manager/main.go`
3. Update gateway decision logs in:
   - `backend/cmd/sms-gateway/handlers.go`
   - `backend/cmd/push-gateway/handlers.go`
4. Update gateway core provider panic logs in:
   - `backend/sms_gateway.go`
   - `backend/push_gateway.go`
5. Update adapter logs in:
   - `backend/adapter/model_provider_call.go`
   - `backend/adapter/default_provider_call.go`
   - `backend/adapter/infobip_provider_call.go`
   - `backend/adapter/karix_provider_call.go`
   - `backend/adapter/sms24x7_provider_call.go`
   - `backend/adapter/push_fcm_provider_call.go`
6. Update SubmissionManager lifecycle and lease logs in:
   - `backend/submissionmanager/attempts.go`
   - `backend/submissionmanager/leader_runner.go`
7. Add `setu_to_setu` gateway-call evidence in `backend/cmd/submission-manager/executor.go`.
8. Add parser/normalizer implementation and tests under `backend/logging`.
9. Run focused tests from `backend/`:

   go test ./cmd/sms-gateway ./cmd/push-gateway ./cmd/submission-manager ./adapter ./submissionmanager ./...

## Validation and Acceptance

Acceptance is met when:

- submission decision/evidence logs are emitted through `slog` (not `log.Printf`) in modified submission files,
- emitted records include required base and profile fields for representative submission events,
- gateway and adapter behavior remains semantically unchanged (same normalized outcomes),
- SubmissionManager attempt and lease events remain present with structured fields required by spec,
- parser/normalizer tests cover valid/invalid fixtures including profile-specific validation and redaction-sensitive rejection,
- focused tests pass for modified packages.

## Idempotence and Recovery

Edits are code-only and additive around logging output shape, with no schema or persistence migrations. If a migration step introduces regressions, revert the affected package-level logging changes while keeping the shared logging package intact, then reapply file-by-file. This allows safe retries without state cleanup.

## Artifacts and Notes

Executed from `backend/`:

  go test ./cmd/sms-gateway ./cmd/push-gateway ./cmd/submission-manager ./adapter ./submissionmanager ./logging

Result:

  ok  	gateway/cmd/sms-gateway
  ok  	gateway/cmd/push-gateway
  ok  	gateway/cmd/submission-manager
  ok  	gateway/adapter
  ok  	gateway/submissionmanager
  ok  	gateway/logging

Executed from `backend/`:

  go test ./...

Result: all packages passed.

Executed from `backend/` after demoting `leader_renewed` severity:

  go test ./submissionmanager ./logging

Result:

  `gateway/submissionmanager` failed in this environment because local SQL Server was not reachable on `localhost:1433`.
  `gateway/logging` passed.

Executed from `backend/` as deterministic fallback validation in this environment:

  go test ./submissionmanager -run '^$' && go test ./logging

Result:

  `gateway/submissionmanager` compiled successfully (`[no tests to run]`), and `gateway/logging` tests passed.

Executed from `backend/` after demoting `leader_acquire_not_granted` severity:

  go test ./submissionmanager -run '^$'

Result:

  `gateway/submissionmanager` compiled successfully (`[no tests to run]`), no behavior regressions expected for runtime logging paths.

## Interfaces and Dependencies

- Use Go stdlib `log/slog`; do not add third-party logging libraries.
- Keep existing provider and manager interfaces unchanged; logging changes must not alter execution contracts.
- Parser/normalizer APIs should be small and explicit, returning typed validation errors suitable for tests.

---

Change log:
- 2026-02-13 / Codex: Created execplan for submission bounded context structured logging rollout aligned to `specs/logging-standard.md`.
- 2026-02-13 / Codex: Implemented rollout across submission gateways/adapters/manager/executor, added parser+tests, and recorded validation evidence.
- 2026-02-13 / Codex: Demoted high-frequency `leader_renewed` event severity from `Info` to `Debug` in `backend/submissionmanager/leader_runner.go`.
- 2026-02-13 / Codex: Demoted `leader acquire not granted` event severity from `Warn` to `Debug` in `backend/submissionmanager/leader_runner.go`.
