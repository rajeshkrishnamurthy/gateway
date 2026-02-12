# Implement SubmissionManager Swagger compatibility contract

This execplan is a living document. The sections `Progress`, `Surprises & Discoveries`, `Decision Log`, and `Outcomes & Retrospective` must be kept up to date as work proceeds.

This plan must be maintained in accordance with `backend/PLANS.md`.

EXECPLAN-READY

## Purpose / Big Picture

After this change, the SubmissionManager bounded context will have one canonical OpenAPI contract that Swagger tooling can consume, and we will have deterministic tests that fail when runtime routes, status codes, or media types drift away from that contract. A developer can verify this by opening the OpenAPI artifact in Swagger UI-compatible tooling and by running SubmissionManager tests that assert route-to-contract parity.

## Progress

- [x] (2026-02-11 17:46Z) Read mode constraints, `backend/PLANS.md`, and the frozen Swagger compatibility spec.
- [x] (2026-02-11 17:47Z) Authored this execplan and marked it `EXECPLAN-READY`.
- [x] (2026-02-11 17:49Z) Added canonical OpenAPI artifact at `specs/submission-tracking/submission-manager-openapi.yaml` covering all in-scope SubmissionManager-owned operations.
- [x] (2026-02-11 17:50Z) Added deterministic contract tests in `backend/cmd/submission-manager/openapi_contract_test.go` for OpenAPI structure/coverage and runtime drift checks.
- [x] (2026-02-11 17:51Z) Aligned runtime with documented media types by setting explicit `text/plain` response headers for `GET /healthz` and `GET /readyz` in `backend/cmd/submission-manager/handlers.go`.
- [x] (2026-02-11 17:51Z) Ran `go test ./cmd/submission-manager -count=1` from `backend/` and captured evidence.
- [x] (2026-02-11 17:51Z) Updated this execplan with outcomes, discoveries, and implementation decisions.

## Surprises & Discoveries

- Observation: `GET /healthz` and `GET /readyz` returned 200 with no explicit `Content-Type` header, which caused the runtime-vs-contract drift test to fail for media type parity.
  Evidence: Initial test failure: `expected media type "text/plain", got ""` in `TestSubmissionManagerOpenAPIRuntimeDriftCheck`.

## Decision Log

- Decision: Keep the OpenAPI artifact in the path mandated by the spec: `specs/submission-tracking/submission-manager-openapi.yaml`.
  Rationale: The frozen spec explicitly defines this path as canonical.
  Date/Author: 2026-02-11 / Codex.
- Decision: Store the OpenAPI artifact as JSON content in a `.yaml` file.
  Rationale: JSON is valid YAML and can be parsed with the Go standard library, enabling deterministic tests without adding dependencies.
  Date/Author: 2026-02-11 / Codex.
- Decision: Add runtime contract-drift tests that assert observed status/media types are declared in the OpenAPI responses.
  Rationale: The spec requires deterministic detection of route/response drift, not only static document checks.
  Date/Author: 2026-02-11 / Codex.
- Decision: Set explicit `text/plain; charset=utf-8` headers in health and ready handlers.
  Rationale: This preserves existing body/status behavior while making non-JSON response media types deterministic and aligned with the OpenAPI contract.
  Date/Author: 2026-02-11 / Codex.

## Outcomes & Retrospective

The SubmissionManager bounded context now has a canonical OpenAPI 3.0 contract at `specs/submission-tracking/submission-manager-openapi.yaml` covering all required owned operations and excluding non-owned delivery routes. Compatibility tests were added to validate both static contract completeness and runtime parity for route status/media behavior.

During implementation, one drift issue was discovered: successful `/healthz` and `/readyz` responses lacked explicit content types. This was corrected in handlers to produce deterministic `text/plain` responses. Package tests now pass with the new compatibility checks enabled.

## Context and Orientation

SubmissionManager HTTP routes are registered in `backend/cmd/submission-manager/routes.go`. Behavior lives in `backend/cmd/submission-manager/handlers.go` and `backend/cmd/submission-manager/ui.go`. JSON response shapes are defined in `backend/cmd/submission-manager/responses.go` and `backend/cmd/submission-manager/errors.go`.

The frozen spec `specs/submission-tracking/submission-manager-swagger-compatibility.md` requires:

- an OpenAPI 3.0.x contract file at `specs/submission-tracking/submission-manager-openapi.yaml`,
- exact coverage for SubmissionManager-owned operations (`/healthz`, `/readyz`, `/metrics`, `/v1/intents`, `/v1/intents/{intentId}`, `/v1/intents/{intentId}/history`, and `/ui/history` when UI is enabled),
- explicit non-ownership of delivery routes,
- deterministic verification that runtime behavior and contract remain aligned.

The existing tests in `backend/cmd/submission-manager/main_test.go` already cover many route behaviors. We will add focused compatibility tests that load the OpenAPI artifact and assert parity against runtime handlers.

## Plan of Work

First create the OpenAPI contract file with OpenAPI 3.0.x metadata, paths, methods, parameters, request/response schemas, and media types aligned with current handler behavior. Keep the contract semantics identical to existing runtime behavior and existing submission specs.

Then add tests under `backend/cmd/submission-manager` that:

1. parse and validate the OpenAPI artifact structure and required operation set,
2. enforce exact in-scope owned operations and exclusion of non-owned delivery routes,
3. execute runtime handler scenarios and assert each observed status/media type is declared in the OpenAPI contract,
4. verify `/ui/history` route behavior both when UI is disabled and enabled.

Finally run package tests and record outputs in this plan.

## Concrete Steps

From repository root:

1. Create `specs/submission-tracking/submission-manager-openapi.yaml` with OpenAPI 3.0.x content describing all in-scope endpoints and schemas.
2. Add `backend/cmd/submission-manager/openapi_contract_test.go` with compatibility tests.
3. Run tests from `backend/`:

    go test ./cmd/submission-manager -count=1

4. Update this execplan sections (`Progress`, `Surprises & Discoveries`, `Decision Log`, `Outcomes & Retrospective`) with final evidence and notes.

## Validation and Acceptance

Acceptance is met when all of the following are true:

- `specs/submission-tracking/submission-manager-openapi.yaml` exists and declares OpenAPI `3.0.x`.
- The contract declares exactly the required SubmissionManager-owned operations and excludes delivery routes.
- Tests prove runtime route/status/media behavior is represented in the OpenAPI responses.
- `/ui/history` behavior is validated for both disabled and enabled UI registration states.
- `go test ./cmd/submission-manager -count=1` passes.

## Idempotence and Recovery

These edits are additive and safe to re-run. If a test fails, fix either the runtime route contract or the OpenAPI artifact so they converge; do not bypass drift failures. Re-running `go test ./cmd/submission-manager -count=1` is safe.

## Artifacts and Notes

Test evidence:

    $ (cd backend && go test ./cmd/submission-manager -count=1)
    ok      gateway/cmd/submission-manager  0.538s

## Interfaces and Dependencies

No new runtime dependencies are required. Use existing Go standard library packages in tests (`encoding/json`, `net/http`, `net/http/httptest`, `os`, `path/filepath`, `strings`, `testing`).

The compatibility tests must exercise existing handler wiring via:

- `newMux(...)`,
- `handleHealthz`, `handleReadyz`, `handleMetrics`,
- `apiServer.handleSubmit`, `apiServer.handleGet`,
- `managerUIServer.handleHistory`.

Plan update note: Added implementation results, drift discovery, response-header correction decision, and final test evidence after completing the OpenAPI artifact and compatibility tests.
