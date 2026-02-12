# Generate SubmissionManager OpenAPI from code and expose live docs endpoint

This execplan is a living document. The sections `Progress`, `Surprises & Discoveries`, `Decision Log`, and `Outcomes & Retrospective` must be kept up to date as work proceeds.

This plan must be maintained in accordance with `backend/PLANS.md`.

EXECPLAN-READY

## Purpose / Big Picture

After this change, SubmissionManager will no longer depend on a hand-maintained OpenAPI artifact as the source of truth. Instead, OpenAPI will be generated from Go code, and the service will expose a live docs experience at `/docs` backed by `/openapi.json`. Users can verify this by starting the service, opening `/docs`, and seeing the live contract rendered from the runtime-generated OpenAPI document.

## Progress

- [x] (2026-02-11 17:58Z) Created this execplan for the OpenAPI-from-code and live docs scope.
- [x] (2026-02-12 02:07Z) Added code-owned OpenAPI document builder package and generation API in `backend/submissionmanagerapi/openapi.go`.
- [x] (2026-02-12 02:07Z) Added generator command in `backend/cmd/submission-manager-openapi/main.go`.
- [x] (2026-02-12 02:08Z) Added `/openapi.json` and `/docs` routes and handlers in submission-manager.
- [x] (2026-02-12 02:09Z) Updated tests to enforce generated-artifact parity and live docs endpoint behavior.
- [x] (2026-02-12 02:09Z) Regenerated `specs/submission-tracking/submission-manager-openapi.yaml` from code.
- [x] (2026-02-12 02:10Z) Ran `go test ./cmd/submission-manager -count=1` and `go test ./cmd/submission-manager-openapi -count=1`.
- [x] (2026-02-12 02:10Z) Updated this execplan with discoveries, decisions, outcomes, and evidence.

## Surprises & Discoveries

- Observation: Running `go run ./cmd/submission-manager-openapi` required escalation because the Go build cache path is outside the workspace sandbox.
  Evidence: sandbox error before escalation: `operation not permitted` under `/Users/rajeshk/Library/Caches/go-build/...`.

## Decision Log

- Decision: Introduce a reusable package for SubmissionManager OpenAPI construction and JSON marshaling.
  Rationale: This allows both runtime serving (`/openapi.json`) and artifact generation to share a single source of truth.
  Date/Author: 2026-02-11 / Codex.
- Decision: Keep the generated artifact as JSON content in `submission-manager-openapi.yaml`.
  Rationale: JSON is valid YAML and allows strict parse/compare checks using the Go standard library with no added dependencies.
  Date/Author: 2026-02-12 / Codex.
- Decision: Add `/openapi.json` to expose the generated runtime contract and `/docs` to expose Swagger UI using CDN assets.
  Rationale: This satisfies the live docs endpoint requirement while keeping server code dependency-free.
  Date/Author: 2026-02-12 / Codex.
- Decision: Enforce artifact parity in tests by comparing generated output bytes to the checked-in artifact.
  Rationale: This guarantees the repo artifact is always generated from code and prevents manual drift.
  Date/Author: 2026-02-12 / Codex.

## Outcomes & Retrospective

SubmissionManager now generates OpenAPI from Go code (`backend/submissionmanagerapi/openapi.go`), serves it live at `/openapi.json`, and serves Swagger UI at `/docs`. The checked-in artifact (`specs/submission-tracking/submission-manager-openapi.yaml`) is now regenerated from the same code path via `cmd/submission-manager-openapi`.

Compatibility tests now verify both contract coverage and artifact parity with generated output, and runtime tests verify docs endpoints and route/media/status drift behavior.

## Context and Orientation

SubmissionManager HTTP adapters live in `backend/cmd/submission-manager/`. Route registration is in `routes.go`; handlers are in `handlers.go`; UI fragment handling is in `ui.go`. Existing OpenAPI compatibility checks are in `backend/cmd/submission-manager/openapi_contract_test.go`, and the current canonical artifact is `specs/submission-tracking/submission-manager-openapi.yaml`.

The requested scope adds two runtime endpoints:

- `/openapi.json` for live machine-readable OpenAPI output.
- `/docs` for browser-rendered Swagger UI.

“Generate from code” in this plan means the OpenAPI document is assembled by Go code and marshaled to JSON; file artifacts are generated from that code path, not edited manually.

## Plan of Work

Create a new package `backend/submissionmanagerapi` with data structures and a `BuildOpenAPIDocument` constructor. This package will expose `MarshalOpenAPIJSON` and `SwaggerUIHTML` helpers. Then wire `GET /openapi.json` and `GET /docs` handlers in `backend/cmd/submission-manager/handlers.go` and register routes in `routes.go`.

Add a command `backend/cmd/submission-manager-openapi` that writes the canonical artifact to a path, so spec regeneration is deterministic and repeatable. Update contract tests to verify that the checked-in artifact matches generated output and that runtime route/media/status behavior for docs endpoints is covered.

## Concrete Steps

1. Add `backend/submissionmanagerapi/openapi.go` with document construction/marshaling and docs HTML generation.
2. Add `backend/cmd/submission-manager-openapi/main.go` with `-out` support for writing generated OpenAPI JSON.
3. Update `backend/cmd/submission-manager/routes.go` and `backend/cmd/submission-manager/handlers.go` to expose `/openapi.json` and `/docs`.
4. Extend `backend/cmd/submission-manager/openapi_contract_test.go` and `backend/cmd/submission-manager/main_test.go` for generation parity and docs endpoint behavior.
5. Regenerate artifact:

    cd backend
    go run ./cmd/submission-manager-openapi -out ../specs/submission-tracking/submission-manager-openapi.yaml

6. Run tests:

    cd backend
    go test ./cmd/submission-manager -count=1
    go test ./cmd/submission-manager-openapi -count=1

## Validation and Acceptance

Acceptance criteria for this execution slice:

- `/openapi.json` returns `200 application/json` with OpenAPI 3.0 content generated from code.
- `/docs` returns `200 text/html` and points Swagger UI to `/openapi.json`.
- The artifact `specs/submission-tracking/submission-manager-openapi.yaml` is generated from code and parity-checked by tests.
- SubmissionManager tests and generator command tests pass.

## Idempotence and Recovery

Generator runs are idempotent because they rewrite the same artifact from deterministic marshaled output. Re-running tests is safe. If parity tests fail, regenerate the artifact and re-run tests.

## Artifacts and Notes

Regeneration command:

    $ (cd backend && go run ./cmd/submission-manager-openapi -out ../specs/submission-tracking/submission-manager-openapi.yaml)

Test evidence:

    $ (cd backend && go test ./cmd/submission-manager -count=1)
    ok      gateway/cmd/submission-manager          0.688s

    $ (cd backend && go test ./cmd/submission-manager-openapi -count=1)
    ok      gateway/cmd/submission-manager-openapi  0.472s

## Interfaces and Dependencies

No new third-party Go dependencies are introduced. Runtime docs UI uses Swagger UI assets via CDN URLs in served HTML.

Primary interfaces to add:

- `submissionmanagerapi.BuildOpenAPIDocument() Document`
- `submissionmanagerapi.MarshalOpenAPIJSON() ([]byte, error)`
- `submissionmanagerapi.SwaggerUIHTML(specURL string) string`

Plan update note: Marked all implementation steps complete, recorded sandbox discovery, captured generation/test evidence, and documented final decisions and outcomes.
