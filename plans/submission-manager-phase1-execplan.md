# Phase 1: SubmissionTarget registry and contract mapping

This ExecPlan is a living document. The sections Progress, Surprises & Discoveries, Decision Log, and Outcomes & Retrospective must be kept up to date as work proceeds.

This plan follows `backend/PLANS.md` from the repository root and must be maintained in accordance with it.

## Purpose / Big Picture

After this change, the repository has a formal SubmissionTarget registry that binds each target to a known gateway type and a routing URL, plus a contract definition that captures execution limits such as mode, acceptance window, retry allowance, and terminal outcomes. The intent is to keep roles crisp: gatewayType is code-known (protocol + response semantics), submissionTarget is data-driven (contract constraints), and SubmissionManager owns time and attempts without reinterpreting gateway semantics. SubmissionManager itself is not implemented in Phase 1; this phase only defines and validates the configuration and provides a small, deterministic loader that other phases will consume. Success is observable by loading the sample registry and by tests that accept valid configs and reject invalid ones.

## Progress

- [x] (2026-01-27 19:00Z) Review gateway contracts, config loaders, and ExecPlan conventions to align the Phase 1 registry design with existing patterns.
- [x] (2026-01-27 20:15Z) Define the SubmissionTarget contract schema and document it in `backend/spec/submission-manager.md`.
- [x] (2026-01-27 20:20Z) Add a sample registry config under `backend/conf/submission/submission_targets.json`.
- [x] (2026-01-27 20:45Z) Implement `gateway/submission` registry loader and validator with explicit gateway type binding.
- [x] (2026-01-27 20:50Z) Add unit tests for registry parsing and validation, and update docs to point at the new registry.

## Surprises & Discoveries

- None yet.

## Decision Log

- Decision: Bind `submissionTarget` to `gatewayType` explicitly in the registry, not in the client intent.
  Rationale: SubmissionManager needs deterministic, visible mapping without relying on naming conventions or client-supplied type hints.
  Date/Author: 2026-01-27 / Codex

- Decision: Store the Phase 1 sample registry under `backend/conf/docker/` and keep it overrideable by path.
  Rationale: The MVP runtime is Docker Compose and existing configs for Compose live under `conf/docker/`.
  Date/Author: 2026-01-27 / Codex

- Decision: Use simple, explicit contract fields (mode, policy, maxAcceptanceSeconds/maxAttempts, terminalOutcomes) and avoid retry schedule parameters in Phase 1.
  Rationale: Phase 1 is only about registry and validation; scheduling details belong to later phases once the execution engine exists.
  Date/Author: 2026-01-27 / Codex

- Decision: Validate terminal outcomes against the known gateway taxonomies (sms and push) and reject unknown reasons.
  Rationale: Prevents mismatched contracts from silently drifting from gateway behavior.
  Date/Author: 2026-01-27 / Codex

- Decision: Define terminalOutcomes as gateway-reported outcomes that this contract treats as terminal, not a statement about gateway behavior.
  Rationale: Keeps semantics in SubmissionManager and avoids implying that gateways change behavior based on contract.
  Date/Author: 2026-01-27 / Codex

- Decision: Define maxAcceptanceSeconds as a cumulative wall-clock bound across all attempts, not a per-attempt timeout.
  Rationale: The contract is about overall acceptance, while attempt timeouts remain gateway-level concerns.
  Date/Author: 2026-01-27 / Codex

- Decision: Require terminalOutcomes to be non-empty and to contain only known outcomes for the selected gatewayType.
  Rationale: Keeps contract definitions explicit and avoids silent retry loops on every rejection.
  Date/Author: 2026-01-27 / Codex

- Decision: Require at least one submissionTarget entry in the registry file.
  Rationale: An empty registry is always a misconfiguration and should fail fast.
  Date/Author: 2026-01-27 / Codex

- Decision: Move the sample registry out of `conf/docker/` into `conf/submission/` to reflect a core service config.
  Rationale: SubmissionManager is a core service and the registry is not Docker-specific.
  Date/Author: 2026-01-27 / Codex

- Decision: Introduce an explicit contract policy field (`deadline`, `max_attempts`, `one_shot`) and validate required fields per policy.
  Rationale: Prevents implicit coupling to a single termination rule and keeps contract semantics explicit.
  Date/Author: 2026-01-27 / Codex

## Outcomes & Retrospective

Phase 1 delivered a SubmissionTarget registry spec, a sample registry config under `backend/conf/submission/`, and a `gateway/submission` package that loads and validates registry files with explicit gatewayType binding. Contracts now include an explicit policy field to define termination rules. Unit tests cover valid and invalid configs, and backend docs plus package-level README/AGENTS notes point to the new registry and spec.

## Context and Orientation

The Go module is `gateway` (see `backend/go.mod`). The submission gateways are synchronous and live in `backend/sms_gateway.go` and `backend/push_gateway.go`, which define the request/response schemas and rejection reasons for SMS and push. HTTP entrypoints and config loaders are in `backend/cmd/sms-gateway/main.go` and `backend/cmd/push-gateway/main.go`; they parse JSON configs with full-line `#` comments stripped, use `DisallowUnknownFields`, and validate values explicitly. Docker Compose configs live under `backend/conf/docker/`, and HAProxy frontends provide stable gateway URLs.

Phase 1 does not add any HTTP handlers or runtime services. It introduces a registry and contract definition so later phases can create intents and execute attempts deterministically. The key terms used below are defined in the new spec: a SubmissionIntent is the client's desired submission; submissionTarget selects a contract and is data-driven; gatewayType is code-known and defines protocol + response semantics; and SubmissionManager owns time and attempts without reinterpreting gateway semantics. Terminal outcomes are gateway-reported outcomes that this contract treats as terminal, and maxAcceptanceSeconds is a cumulative wall-clock bound across all attempts when policy is `deadline`.

## Plan of Work

Create a new spec file under `backend/spec/` that defines SubmissionIntent, submissionTarget, gatewayType, contract fields, and the acceptance window semantics. The spec should include the known gateway outcome reasons for SMS and push and explain that the registry provides the explicit submissionTarget to gatewayType binding. The spec must also clarify that terminalOutcomes are gateway-reported outcomes that this contract treats as terminal, and that maxAcceptanceSeconds is a cumulative wall-clock bound across all attempts when policy is `deadline`. Add a sample registry config under `backend/conf/submission/` with at least one SMS and one push target that point at HAProxy frontends. Implement a small `gateway/submission` package that loads the registry from JSON with full-line `#` comment stripping, disallows unknown fields, and validates all contract invariants. Validation should enforce non-empty target names, known gateway types, valid routing URLs, policy-specific required fields, and terminal outcomes that are part of the gateway's known reason set. The package should provide a lookup method that returns a contract for a given submissionTarget without mutating or normalizing the input beyond trimming whitespace.

## Concrete Steps

Step 1: Add `backend/spec/submission-manager.md` documenting the SubmissionTarget registry schema, the gateway outcome taxonomies, and the acceptance window semantics (cumulative wall-clock deadline from intent creation across all attempts). Include explicit definitions separating gatewayType (protocol + response semantics), submissionTarget (data-driven contract constraints), and SubmissionManager (time and attempts only).

Step 2: Add `backend/conf/submission/submission_targets.json` with a minimal set of targets (at least one SMS and one push) and clear, stable naming.

Step 3: Create `backend/submission/registry.go` and `backend/submission/registry_test.go` with config loading, validation, and lookup behavior that mirrors existing config parsing patterns (full-line `#` comments, `DisallowUnknownFields`, explicit error messages).

Step 4: Update `backend/README.md` or a small pointer doc to reference the new registry and spec as the source of truth for submissionTarget to gatewayType binding.

## Validation and Acceptance

From `backend/`, run the unit tests for the new package and the overall module. The tests must include at least one valid config that loads successfully and multiple invalid configs that fail with clear errors (unknown gateway type, unknown terminal outcome, invalid URL, missing target name, or non-positive acceptance window). Example command and expected output:

  go test ./...
  ok   gateway/submission 0.0Xs

Acceptance for Phase 1 is met when the registry loader accepts the sample config and the test suite demonstrates deterministic failures for malformed configs.

## Idempotence and Recovery

These changes are additive and safe to re-run. If validation rules change, update the sample config and tests together to keep `go test` passing. No runtime state is created in Phase 1.

## Artifacts and Notes

The registry JSON supports full-line `#` comments and disallows unknown fields. terminalOutcomes list gateway-reported outcomes that the contract treats as terminal. maxAcceptanceSeconds is a cumulative wall-clock bound across all attempts when policy is `deadline`. A minimal example in the expected shape is shown below for clarity:

  {
    "targets": [
      {
        "submissionTarget": "sms.realtime",
        "gatewayType": "sms",
        "gatewayUrl": "http://localhost:8080",
        "mode": "realtime",
        "policy": "deadline",
        "maxAcceptanceSeconds": 30,
        "terminalOutcomes": ["invalid_request", "invalid_recipient", "invalid_message"]
      },
      {
        "submissionTarget": "push.realtime",
        "gatewayType": "push",
        "gatewayUrl": "http://localhost:8081",
        "mode": "realtime",
        "policy": "deadline",
        "maxAcceptanceSeconds": 30,
        "terminalOutcomes": ["invalid_request", "unregistered_token"]
      }
    ]
  }

## Interfaces and Dependencies

All code must use the Go standard library only and must not depend on `net/http`. The registry package should live at `backend/submission/` and expose a narrow surface that other phases can reuse. Define the following types and functions in `backend/submission/registry.go`:

  type GatewayType string

  const (
    GatewaySMS  GatewayType = "sms"
    GatewayPush GatewayType = "push"
  )

  type ContractPolicy string

  const (
    PolicyDeadline    ContractPolicy = "deadline"
    PolicyMaxAttempts ContractPolicy = "max_attempts"
    PolicyOneShot     ContractPolicy = "one_shot"
  )

  type TargetContract struct {
    SubmissionTarget     string
    GatewayType           GatewayType
    GatewayURL            string
    Mode                  string
    Policy                ContractPolicy
    MaxAcceptanceSeconds  int
    MaxAttempts           int
    TerminalOutcomes      []string
  }

  type Registry struct {
    Targets map[string]TargetContract
  }

  func LoadRegistry(path string) (Registry, error)
  func (r Registry) ContractFor(target string) (TargetContract, bool)

Validation should be explicit and return user-readable errors. The list of allowed terminal outcomes must be defined per gateway type inside this package (derived from the current SMS and push gateway response reasons).

---

Change log: Initial Phase 1 plan drafted to define a SubmissionTarget registry, config schema, and validation rules before implementing execution or HTTP APIs. (2026-01-27 / Codex)
Change log: Tightened terminology separation and clarified terminalOutcomes and maxAcceptanceSeconds semantics. (2026-01-27 / Codex)
Change log: Marked Phase 1 implementation complete and recorded validation decisions. (2026-01-27 / Codex)
Change log: Noted package-level README and AGENTS additions in outcomes. (2026-01-27 / Codex)
Change log: Moved sample registry config to `backend/conf/submission/` to reflect core service ownership. (2026-01-27 / Codex)
Change log: Clarified in spec that submissionTarget is the contract identifier. (2026-01-27 / Codex)
Change log: Added explicit contract policy and updated registry validation accordingly. (2026-01-27 / Codex)
