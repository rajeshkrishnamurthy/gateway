# Phase 3b: HTTP service for SubmissionManager

This ExecPlan is a living document. The sections Progress, Surprises & Discoveries, Decision Log, and Outcomes & Retrospective must be kept up to date as work proceeds.

This plan follows `backend/PLANS.md` from the repository root and must be maintained in accordance with it.

## Purpose / Big Picture

After this change, a client can create or query a SubmissionIntent over HTTP without changing any existing SubmissionManager semantics. The service exposes the current intent state, and repeated submissions with the same intentId behave idempotently. The HTTP layer is an adapter only: it delegates to SubmissionManager and does not change policies, retries, or timing. Success is observable by sending an HTTP POST to create an intent, then a GET to retrieve it, and seeing identical results to in-process usage.

## Progress

- [x] (2026-01-29 16:46Z) Review existing SubmissionManager specs and invariants for HTTP mapping.
- [x] (2026-01-29 16:46Z) Draft HTTP API shapes and error mapping in the ExecPlan.
- [x] (2026-01-29 16:46Z) Add a new HTTP service entrypoint under `backend/cmd/` wired to registry and SQL.
- [x] (2026-01-29 16:46Z) Implement HTTP handlers that delegate to SubmissionManager with minimal validation.
- [x] (2026-01-29 16:46Z) Add handler and integration tests using the SQL test DB helper.
- [x] (2026-01-29 16:46Z) Update specs and README to document the HTTP surface and how to run it.

## Surprises & Discoveries

None yet.

## Decision Log

- Decision: HTTP responses exclude contract snapshots and omit payload/attempt details; only intentId, submissionTarget, createdAt, status, completedAt (when terminal), rejectedReason (when rejected), and exhaustedReason (when exhausted) are exposed.
  Rationale: Contract snapshots are internal execution state and the client-facing surface stays compact while preserving intent semantics.
  Date/Author: 2026-01-29 / Codex

- Decision: HTTP endpoints are POST `/v1/intents` (create-or-query) and GET `/v1/intents/{intentId}` (query).
  Rationale: Keeps the adapter surface minimal and aligned with intent semantics.
  Date/Author: 2026-01-29 / Codex

- Decision: Gateway executor maps gatewayType to `/sms/send` and `/push/send`, parses JSON responses only on 2xx, and treats non-2xx or decode failures as attempt errors.
  Rationale: Keeps HTTP transport errors distinct from normalized outcomes while preserving gateway semantics.
  Date/Author: 2026-01-29 / Codex

- Decision: Unknown submissionTarget errors are surfaced via a typed manager error and mapped to HTTP 400 invalid_request.
  Rationale: Keeps validation behavior explicit without adding business logic to handlers.
  Date/Author: 2026-01-29 / Codex

## Outcomes & Retrospective

Phase 3b exposes SubmissionManager over HTTP as a thin adapter while preserving Phase 1–3a semantics. The HTTP service delegates to the manager for submit/query, omits contract snapshots, and returns the single intent response shape (intentId, submissionTarget, createdAt, status, completedAt when terminal, rejectedReason when rejected, exhaustedReason when exhausted). Tests validate idempotency and error mapping; docs describe the HTTP surface and run instructions.

## Context and Orientation

SubmissionManager core logic lives in `backend/submissionmanager/manager.go` and uses SQL Server via `backend/submissionmanager/store.go`. The SubmissionTarget registry is file-based and loaded via `backend/submission/registry.go` from `backend/conf/submission/submission_targets.json`. Phase 3a already established that intent state, attempts, and scheduling metadata are persisted in SQL Server and that SubmissionManager owns time and retries. There is no HTTP surface yet for SubmissionManager.

This phase adds a new HTTP entrypoint under `backend/cmd/` that exposes create-or-query and query operations. The cmd-layer rules in `backend/cmd/AGENTS.md` require that HTTP routing and handlers live in the same `cmd/<service>` package (they may be split across files), that `main.go` is limited to wiring and startup, that core packages do not depend on `net/http`, and that handlers are thin adapters that call existing core logic without adding business rules. The new service must read the registry from disk, open a SQL Server connection, construct a SubmissionManager instance, start its Run loop, and serve HTTP requests that map directly to `SubmitIntent` and `GetIntent`.

Key files and roles:

- `backend/submissionmanager/manager.go` defines SubmissionManager, Intent, Attempt, and the `SubmitIntent`/`GetIntent` APIs.
- `backend/submissionmanager/store.go` persists intents and attempts to SQL Server.
- `backend/submission/registry.go` loads and validates SubmissionTarget contracts.
- `backend/conf/submission/submission_targets.json` is the registry config to load.
- `backend/cmd/` is where HTTP servers are defined; route registration and handlers must live in `main.go` for the new command.
- `specs/submission-manager.md` is the canonical semantics and must be updated for the HTTP surface.

Important terms:

- SubmissionIntent: the client request containing intentId, submissionTarget, and payload.
- SubmissionTarget contract: the registry entry that defines policy (deadline, max_attempts, one_shot) and terminal outcomes.
- Attempt: a single submission try recorded with timestamps and normalized gateway outcome.
- Intent status: accepted, rejected, exhausted, or pending; terminal intents are immutable.

## Plan of Work

Create a new command at `backend/cmd/submission-manager/` with a `main.go` that opens SQL Server, loads the registry, constructs a SubmissionManager, and starts an HTTP server. The HTTP surface will expose two endpoints: a POST that submits an intent (create-or-query) and a GET that fetches an intent by intentId. The HTTP handlers will parse input, call `SubmitIntent` or `GetIntent`, and return the single JSON response shape for intents without applying any business rules or policy logic. Idempotency conflicts from `SubmitIntent` will map to HTTP 409, unknown intent to HTTP 404, and validation errors to HTTP 400. All other errors will map to HTTP 500. Contract files remain file-based and immutable; HTTP will not expose any contract mutation endpoints.

The service will accept minimal configuration via flags or environment variables: listen address, registry path, and SQL Server connection parameters. The chosen configuration format must be documented in a README for the new command and in `backend/README.md`. SQL Server remains the source of truth; handlers must not bypass the SubmissionManager or write directly to the database.

Tests will be added with `net/http/httptest` to validate submit/query behavior, idempotency semantics, and error mapping. Tests will use the SQL Server test DB helper in `backend/submissionmanager/testdb_test.go` to ensure the HTTP layer persists data exactly like the in-process manager.

## Concrete Steps

Work from the repository root unless noted. The steps below are written so they can be performed in order with minimal guesswork.

1) Read `specs/submission-manager.md` and `backend/submissionmanager/README.md` to confirm the exact Intent/Attempt fields and status semantics. Record the HTTP request/response shapes in this ExecPlan as the authoritative mapping.

2) Add `backend/cmd/submission-manager/main.go` that does the following:
   - Parse configuration (address, registry path, SQL connection).
   - Load the registry JSON from the registry path.
   - Open SQL Server using the existing driver and connect with a reasonable timeout.
   - Construct `submissionmanager.Manager` and start `Run` in a goroutine with a cancelable context.
   - Register HTTP routes and handlers in `main.go` and start the server.

3) Define the HTTP API shapes in the `cmd/submission-manager` package using Go structs for JSON encoding. The response should include intentId, submissionTarget, createdAt, status, completedAt (when terminal), rejectedReason (when rejected), and exhaustedReason (when exhausted) but must not expose the contract snapshot or payload. The request for submit must include intentId, submissionTarget, and payload.

4) Implement the handlers as thin adapters in the `cmd/submission-manager` package:
   - POST /v1/intents: decode JSON, validate required fields, call `SubmitIntent`, and encode the Intent response.
   - GET /v1/intents/{intentId}: call `GetIntent` and return 404 if not found.
   - Map idempotency conflicts to HTTP 409 with a structured error body.

5) Add tests for the HTTP layer using `httptest`:
   - Submit intent, then submit again with same payload and assert idempotent response.
   - Submit intent with conflicting payload and assert 409.
   - Query unknown intent and assert 404.

6) Update `specs/submission-manager.md` with a new HTTP API section that documents endpoints, request/response JSON, and error mapping. Update `backend/README.md` to show how to run the new HTTP service.

7) Run tests from `backend/`:

   go test ./...

Expected result: all tests pass and the new HTTP tests validate idempotent semantics.

## Validation and Acceptance

Start SQL Server via Docker (already required by Phase 3a). Run the new submission-manager HTTP service and verify:

- POST /v1/intents with a valid intent returns HTTP 200 and a JSON body containing status "pending".
- Repeating the same POST returns HTTP 200 with the same intent state and does not create a new attempt.
- POST with the same intentId but a different payload returns HTTP 409 with an error describing the conflict.
- GET /v1/intents/{intentId} returns the current intent state, or 404 if unknown.

These must match the same behavior as direct `SubmitIntent`/`GetIntent` usage and must not alter retry timing or policy outcomes.

## Idempotence and Recovery

The HTTP adapter must remain stateless and idempotent. It should be safe to restart the service; SubmissionManager will rebuild its schedule from SQL Server on startup. The HTTP layer must not expose contract snapshots in its responses. If the HTTP server fails to start due to SQL connection errors, surface a clear startup error and exit without partially serving requests.

Schema changes are out of scope for Phase 3b. If schema scripts are rerun, they should remain idempotent as in Phase 3a.

## Artifacts and Notes

Expected HTTP request example (indentation indicates the JSON body, not a code fence):

  POST /v1/intents
  {
    "intentId": "intent-1",
    "submissionTarget": "sms.realtime",
    "payload": {"to":"+1...","message":"hello"}
  }

Expected HTTP response example:

  200 OK
  {
    "intentId": "intent-1",
    "submissionTarget": "sms.realtime",
    "createdAt": "2026-01-29T00:00:00Z",
    "status": "pending"
  }

Error response format (example for idempotency conflict):

  409 Conflict
  {
    "error": {
      "code": "idempotency_conflict",
      "message": "intentId already exists with different payload",
      "details": {
        "intentId": "intent-1",
        "existingTarget": "sms.realtime",
        "incomingTarget": "sms.realtime"
      }
    }
  }

## Interfaces and Dependencies

New HTTP command:

- Path: `backend/cmd/submission-manager/` with `main.go` plus optional `routes.go`/`handlers.go`.
- `main.go` must only wire dependencies and start the server.
- Routes and handlers must live in the same `cmd/submission-manager` package (per `backend/cmd/AGENTS.md`).
- Must construct and own a `submissionmanager.Manager` and call its `Run` method in a goroutine.
- Must not add business logic; handlers are adapters only.

Core APIs to use:

- `submission.Registry` and `submission.LoadRegistry(path)` for contract loading.
- `submissionmanager.NewManager(reg, exec, clock, db)` to construct the manager. The HTTP service will provide a real executor implementation or use existing gateway execution wiring if already available; if no executor exists yet, the plan must define a stub and document it explicitly.
- `(*submissionmanager.Manager).SubmitIntent(ctx, intent)` to create or query intents.
- `(*submissionmanager.Manager).GetIntent(intentId)` to fetch intents.

The HTTP layer must not depend on or mutate `backend/submission/registry.go` beyond reading the registry file, and must not bypass the SQL store.

---

Change log:
- Phase 3b ExecPlan created to add an HTTP adapter for SubmissionManager while preserving Phase 1–3a invariants. (2026-01-29 / Codex)
- Updated to reflect handler placement rule changes and removal of contract snapshots from HTTP responses; added executor mapping decisions and completion status. (2026-01-29 / Codex)
- Updated response shape to include completedAt and rejectedReason while keeping responses contract-free. (2026-01-29 / Codex)
- Updated HTTP response fields to use completedAt/rejectedReason/exhaustedReason and removed finalOutcome from the API surface. (2026-01-30 / Codex)
