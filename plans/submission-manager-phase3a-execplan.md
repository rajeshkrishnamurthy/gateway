# Phase 3a: SQL Server durability for SubmissionManager

This execplan is a living document. The sections Progress, Surprises & Discoveries, Decision Log, and Outcomes & Retrospective must be kept up to date as work proceeds.

This plan follows `backend/PLANS.md` from the repository root and must be maintained in accordance with it.

## Purpose / Big Picture

After this change, SubmissionManager persists intents, attempts, and scheduling metadata in SQL Server so intent state survives process restarts. A developer can start SQL Server via Docker, run the SubmissionManager tests, and observe that idempotency and retry behavior remain consistent across restarts.

## Progress

- [x] Review existing SubmissionManager contracts and enumerate all fields that must be persisted.
- [x] Decide the SQL driver and connection configuration shape for local and production use.
- [x] Define the SQL schema for intents, attempts, and scheduling metadata.
- [x] Implement a SQL-backed store and integrate it with SubmissionManager execution.
- [x] Add integration tests against local SQL Server.
- [x] Update specs and docs to reflect persistence and setup.

## Surprises & Discoveries

- SQL Server DATETIME2 values return without timezone, so comparisons against fake-clock UTC values were offset by local time. Normalizing stored and loaded timestamps as UTC was required to preserve deadline semantics and retry scheduling.
- SQL Server requires QUOTED_IDENTIFIER and ANSI_NULLS ON when creating filtered indexes; schema script now sets these before the nextAttempt index.

## Decision Log

- Decision: Use SQL Server in Docker on macOS for Phase 3a persistence testing and development.
  Rationale: Matches target customer deployments while staying compatible with local development.
  Date/Author: 2026-01-29 / Codex

- Decision: Store payload as raw bytes plus a payload hash in SQL Server.
  Rationale: Raw bytes are required for retries after restart; hash enables strict idempotency checks.
  Date/Author: 2026-01-29 / Codex

- Decision: Record each attempt and the corresponding intent state update in a single SQL transaction.
  Rationale: Preserves atomic intent history and avoids inconsistent terminal states.
  Date/Author: 2026-01-29 / Codex

- Decision: Phase 3a persists only resolved contract snapshots per intent; contract masters remain file-based.
  Rationale: Keeps submissionTarget contracts immutable and avoids coupling to registry storage changes.
  Date/Author: 2026-01-29 / Codex

- Decision: Persist nextAttemptAt in SQL and rebuild the in-memory schedule from persisted state on startup.
  Rationale: SubmissionManager remains the sole scheduler while durability survives restarts.
  Date/Author: 2026-01-29 / Codex

- Decision: Phase 3a assumes a single SubmissionManager process with no worker claiming or leasing.
  Rationale: Keeps scope limited to single-process durability and avoids distributed coordination.
  Date/Author: 2026-01-29 / Codex

- Decision: Treat persisted timestamps as UTC by storing UTC values and normalizing DATETIME2 reads to UTC.
  Rationale: Avoids local timezone skew when comparing deadlines and scheduling retries.
  Date/Author: 2026-01-29 / Codex

## Outcomes & Retrospective

Phase 3a persistence is in place. SubmissionManager now stores intents, attempts, and scheduling metadata in SQL Server, rebuilds its in-memory queue on startup, and enforces idempotency across restarts. Tests cover persistence and restart scheduling. Docs and specs reflect SQL durability and local setup.

## Context and Orientation

Phase 2 introduced the in-memory SubmissionManager execution engine in `backend/submissionmanager/`. The SubmissionTarget registry remains in `backend/submission/`. The current engine keeps all intent state in memory and is lost on restart. SQL Server is now running locally via Docker (`mssql` service in `docker-compose.yml`) with credentials stored in `backend/.env`. Phase 3a will add persistence for intents, attempts, and scheduling metadata without introducing an HTTP API.

Key files:

- `backend/submissionmanager/manager.go` — current in-memory engine, queues, policy evaluation.
- `backend/submission/registry.go` — contract lookup and validation.
- `specs/submission-manager.md` — canonical semantics and Phase 2 behavior.
- `backend/README.md` and `README.md` — local SQL Server connection info.

## Plan of Work

Introduce a SQL-backed store for intent and attempt state, and move the scheduling queue to query-based due selection rather than an in-memory heap. Define a schema that captures intent identity, contract snapshot, payload, status, attempts, and next attempt timing. Wire the manager to read/write from SQL Server and ensure idempotency is enforced at the database level. Add integration tests that run against the Docker SQL Server to confirm behavior across restarts.

## Concrete Steps

1) Decide on the Go SQL Server driver (likely `github.com/microsoft/go-mssqldb`) and define connection configuration (env vars and DSN). Document the configuration in `backend/README.md` and the spec.

2) Create a schema migration directory (for example `backend/conf/sql/submissionmanager/`) and add SQL scripts to create tables for:
   - `submission_intents` (intentId, submissionTarget, payload bytes, payload_hash, contract snapshot, status, finalOutcome, exhaustedReason, createdAt, updatedAt, nextAttemptAt)
   - `submission_attempts` (intentId, attemptNumber, timestamps, gatewayOutcome, error)
   - No worker claiming or leasing fields (single-process scheduling only).

3) Implement a SQL store package (new package under `backend/submissionmanager/store/` or similar) with operations:
   - Insert intent (idempotent on intentId with conflict detection)
   - Fetch intent by id
   - Append attempt, update intent status/final outcome, and compute nextAttemptAt in a single transaction
   - Fetch due intents for execution without worker claiming or leasing

4) Update `backend/submissionmanager/manager.go` to use the SQL store instead of the in-memory map/queue. Ensure retries are scheduled by updating `nextAttemptAt` and do not execute after deadline. Rebuild the in-memory scheduling queue from persisted nextAttemptAt values on startup.

5) Add integration tests that:
   - Apply schema migrations to a test database
   - Submit an intent, restart manager (recreate), and confirm state is retained
   - Verify idempotency conflict and retry behavior with persisted attempts

6) Update `specs/submission-manager.md` with persistence semantics, and update `specs/README.md` or `backend/README.md` for the SQL Server setup and schema location.

## Validation and Acceptance

From `backend/`, run:

  go test ./...

Acceptance is met when:

- A manager restart does not lose intent state.
- Idempotent submissions return the same intent record across restarts.
- A deadline-based intent does not schedule retries beyond the acceptance deadline.
- SQL Server schema creation scripts are in repo and referenced in docs.

## Idempotence and Recovery

Schema migrations must be repeatable or idempotent, and the store must handle restarts gracefully. If a migration fails, document rollback steps (drop/recreate test DB) in the execplan.

## Artifacts and Notes

- Schema scripts should be simple, explicit SQL files in `backend/conf/sql/submissionmanager/`.
- Tests should clean up by dropping the test database or truncating tables.

## Interfaces and Dependencies

- Use Go standard library `database/sql` with the selected SQL Server driver.
- New types or interfaces should remain minimal; avoid unnecessary abstractions.
- SubmissionManager retains its existing public API (`SubmitIntent`, `GetIntent`, `Run`).

---

Change log: Phase 3a execplan created to add SQL Server durability for SubmissionManager. (2026-01-29 / Codex)
