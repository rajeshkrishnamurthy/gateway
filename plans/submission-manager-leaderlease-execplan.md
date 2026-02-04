# SubmissionManager Leader Lease (SQL Server)

This execplan is a living document. The sections Progress, Surprises & Discoveries, Decision Log, and Outcomes & Retrospective must be kept up to date as work proceeds.

This plan follows `backend/PLANS.md` from the repository root and must be maintained in accordance with it.

## Purpose / Big Picture

After this change, multiple SubmissionManager instances can sit behind HAProxy for high-availability HTTP while ensuring exactly one executor runs attempts at any time. The leader is elected and renewed via a SQL Server lease, and the leader incrementally refreshes its in-memory schedule from SQL so intents submitted to followers are picked up quickly. Success is observable by running two instances, seeing only one execute attempts, forcing lease expiry to observe failover, and confirming `/readyz` shows leader or follower status while both continue serving HTTP.

## Progress

- [x] (2026-02-03 20:10Z) Read `specs/submission-manager-leaderlease.md` and repository context; draft execplan.
- [x] (2026-02-03 21:05Z) Update SQL schema for leader leases and intent schedule watermark.
- [x] (2026-02-03 21:20Z) Implement lease acquisition/renewal, leader gating, and schedule refresh loops.
- [x] (2026-02-03 21:25Z) Fence executor-side writes by lease_epoch and holder_id.
- [x] (2026-02-03 21:35Z) Integrate leader status into the HTTP service and `/readyz` output.
- [x] (2026-02-03 21:45Z) Add SQL-backed tests for leader election, failover, lease loss, and schedule refresh.
- [x] (2026-02-03 21:50Z) Update `backend/README.md` to reflect multi-instance execution configuration and readiness output.
- [x] (2026-02-03 22:00Z) Run and validate tests (`go test ./...`).

## Surprises & Discoveries

None yet.

## Decision Log

- Decision: Implement a submissionmanager-owned leader runner that owns lease acquisition/renewal, schedule rebuild/refresh, and starts/stops the executor loop, exposing a read-only `LeaseStatus` for HTTP readiness.
  Rationale: The lease is part of execution correctness and needs to live with the manager so tests can exercise lease loss and fencing without HTTP plumbing.
  Date/Author: 2026-02-03 / Codex

- Decision: Add a `scheduled` map keyed by intentId and treat heap entries as potentially stale; skip heap entries that do not match the latest scheduled due time in the map.
  Rationale: The heap cannot delete arbitrary entries, but this keeps schedule refresh idempotent and avoids duplicate executions when rows are observed repeatedly.
  Date/Author: 2026-02-03 / Codex

- Decision: Keep `created_at` and `updated_at` driven by the manager clock for existing semantics and tests, and add a separate `last_modified_at` column driven by `SYSUTCDATETIME()` to power schedule refresh.
  Rationale: The spec requires a SQL-time watermark while current tests and deadline behavior rely on the manager clock; decoupling avoids breaking existing semantics.
  Date/Author: 2026-02-03 / Codex

- Decision: Do not modify documents under `specs/` during EXEC; limit documentation updates to `backend/README.md`.
  Rationale: `AGENTS.md` forbids modifying spec documents in EXEC.
  Date/Author: 2026-02-03 / Codex

- Decision: Include `lease_name` in the lease fencing token so executor-side writes are fenced against the configured lease row.
  Rationale: Lease name is configurable; fencing must target the same lease row used for acquire/renew.
  Date/Author: 2026-02-03 / Codex

## Outcomes & Retrospective

Leader lease acquisition/renewal, schedule rebuild/refresh, fenced executor writes, and readiness reporting are implemented end-to-end with SQL-backed tests. `go test ./...` passes in `backend/`. (2026-02-03 / Codex)

## Context and Orientation

SubmissionManager core execution lives in `backend/submissionmanager/`. It owns intent lifecycle, attempt execution, and persistence to SQL Server via the `sqlStore` in `backend/submissionmanager/store_*.go`. The HTTP service for SubmissionManager is `backend/cmd/submission-manager/`, which constructs a `submissionmanager.Manager`, starts it, and exposes `/v1/intents`, `/healthz`, `/readyz`, and `/metrics`.

Key terms used in this plan:

Leader lease: a single SQL row in `submission_manager_leases` representing which instance is allowed to execute attempts. It includes `holder_id`, `lease_epoch`, and `expires_at` driven by SQL Server time.

Follower: an instance that serves HTTP but does not execute attempts or maintain an in-memory schedule.

Fencing: every executor-side write (intent updates and attempt inserts) must check that the current `(holder_id, lease_epoch)` is still held. If a fenced write affects 0 rows, the instance must drop leadership immediately.

Schedule refresh: the leader’s periodic query against `submission_intents` ordered by `(last_modified_at, intent_id)` that picks up new or rescheduled intents submitted by followers. Refresh must be idempotent.

The authoritative behavior is defined in `specs/submission-manager-leaderlease.md`. Documents under `specs/` are frozen for EXEC; do not edit them during this plan.

## Plan of Work

First, extend the SQL schema under `backend/conf/sql/submissionmanager/001_create_schema.sql` to include the `submission_manager_leases` table and a `last_modified_at` column on `submission_intents`. Ensure `last_modified_at` is set on insert and updated on every intent update using `SYSUTCDATETIME()` in the same statement. This column is the watermark for schedule refresh.

Next, add lease persistence helpers to `backend/submissionmanager/` that can acquire or renew the lease using SQL Server time. Implement lease acquisition (insert or update on expiry), renewal (update only if held and not expired), and a lease status reader that exposes `mode`, `holder_id`, `lease_epoch`, and `expires_at` for readiness reporting. Lease operations must use `SYSUTCDATETIME()` and return explicit success/failure with errors surfaced.

Then, update `submissionmanager.Manager` to support leader-only scheduling. This includes: not building the schedule on construction; adding an explicit schedule rebuild method that the leader calls after acquiring the lease; adding a schedule refresh method that loads rows since the composite cursor and updates the in-memory queue; and making `SubmitIntent` enqueue only when the instance is leader. Add leadership checks before dequeueing and before executing an attempt, and ensure executor-side writes (`recordAttempt`, `markExhausted`, `recordWebhookAttempt`) are fenced on `(holder_id, lease_epoch)`.

Introduce a leader runner in `backend/submissionmanager/` that coordinates acquisition, renewal, schedule rebuild, refresh, and executor lifecycle. It should start the executor loop only when leadership is held, stop it immediately on lease loss or SQL errors, and clear the in-memory schedule when leadership drops. The runner should own the refresh cursor `(last_modified_at, intent_id)` and log `leader_acquired`, `leader_renewed`, `leader_lost`, `leader_acquire_failed`, and `leader_renew_failed` with `holder_id`, `lease_epoch`, `expires_at`, and `sql_error` when present.

Update the HTTP service in `backend/cmd/submission-manager/` to construct the leader runner, pass in the lease configuration (including `holder_id`), and expose lease status through `/readyz`. Keep `/readyz` returning HTTP 200 for both leaders and followers, but include role information in the response body.

Finally, add SQL-backed tests in `backend/submissionmanager/` to verify single-leader election, failover after expiry, execution gating without leadership, immediate stop on lease loss, and schedule refresh picking up follower-submitted intents. Update `backend/README.md` to reflect the new multi-instance execution model and configuration.

## Concrete Steps

1) Update the SQL schema in `backend/conf/sql/submissionmanager/001_create_schema.sql` to add the `submission_manager_leases` table and the `last_modified_at` column on `submission_intents`. Use `SYSUTCDATETIME()` for lease timestamps and last_modified_at.

2) Add lease persistence helpers in `backend/submissionmanager/` (new file such as `lease_store.go`) that implement acquire, renew, and read status. Use single-statement SQL with `SYSUTCDATETIME()` and return whether a lease was acquired or renewed.

3) Modify `backend/submissionmanager/manager.go`, `schedule.go`, and related store files to:

   - Add a schedule rebuild method that loads all pending schedule entries from SQL and initializes the refresh cursor.
   - Add a schedule refresh method that queries by `(last_modified_at, intent_id)` and upserts schedule entries without duplicates.
   - Add leader checks before dequeueing and before executing attempts.
   - Fence executor-side writes by `(holder_id, lease_epoch)` using SQL predicates and treat 0-row updates as lease loss.
   - Ensure every update to `submission_intents` sets `last_modified_at = SYSUTCDATETIME()` in the same statement.

4) Implement a leader runner in `backend/submissionmanager/` that owns acquisition, renewal, schedule rebuild/refresh, and executor start/stop. It should expose a `LeaseStatus` getter for HTTP readiness and clear the schedule on leadership loss.

5) Update `backend/cmd/submission-manager/` to:

   - Add flags or env overrides for lease_duration, renew_interval, acquire_interval, schedule_refresh_interval, lease_name, and holder_id (default holder_id uses hostname-pid-rand).
   - Start the leader runner instead of calling `manager.Run` directly.
   - Return `/readyz` content such as `mode=leader holder_id=<id> lease_expires_at=<time>` or `mode=follower holder_id=<id>` while still returning HTTP 200.

6) Add tests under `backend/submissionmanager/` that use `newTestDB` to validate leader election, failover, fencing behavior, and schedule refresh from follower-submitted intents. Keep durations short (sub-second to a few seconds) to keep tests fast and deterministic.

7) Update `backend/README.md` to describe the leader lease model, new configuration, and readiness output.

8) Run tests from `backend/`:

   go test ./...

Expected result: all tests pass, including the new leader-lease tests.

## Validation and Acceptance

The change is accepted when two running instances show only one executing attempts at a time, lease expiry triggers a follower to become leader, and `/readyz` reports the correct role while remaining HTTP-200 for both leaders and followers. SQL-backed tests must cover the five scenarios in `specs/submission-manager-leaderlease.md` and pass consistently when SQL Server is running.

## Idempotence and Recovery

Schema updates must be idempotent and safe to re-run. If a test DB gets wedged, drop and recreate it using the existing `newTestDB` helper flow. Schedule refresh must be idempotent so repeated refresh cycles do not enqueue duplicate attempts.

## Artifacts and Notes

Example readyz body when leader:

  mode=leader holder_id=sm-01 lease_expires_at=2026-02-02T12:00:10Z

Example leader log entry fields:

  leader_acquired holder_id=sm-01 lease_epoch=2 expires_at=2026-02-02T12:00:10Z

## Interfaces and Dependencies

Add these types and signatures in `backend/submissionmanager/` (names may be exported if needed by `cmd/submission-manager`):

  type LeaseConfig struct {
      LeaseName               string
      HolderID                string
      LeaseDuration           time.Duration
      RenewInterval           time.Duration
      AcquireInterval         time.Duration
      ScheduleRefreshInterval time.Duration
  }

  type LeaseStatus struct {
      Mode      string // "leader" or "follower"
      HolderID  string
      LeaseEpoch int64
      ExpiresAt time.Time
  }

  type LeaderRunner struct {
      // owns acquisition/renewal, schedule rebuild/refresh, and executor lifecycle
  }

  func NewLeaderRunner(store *sqlStore, manager *Manager, cfg LeaseConfig) *LeaderRunner
  func (r *LeaderRunner) Run(ctx context.Context)
  func (r *LeaderRunner) Status() LeaseStatus
  func (r *LeaderRunner) IsLeader() bool
  func (r *LeaderRunner) CurrentLease() (holderID string, epoch int64, ok bool)

Update store methods to accept lease fencing where they mutate execution state:

  func (s *sqlStore) recordAttempt(ctx context.Context, fence LeaseFence, intentID string, attempt Attempt, status IntentStatus, finalOutcome GatewayOutcome, exhaustedReason string, nextAttemptAt *time.Time, now time.Time) (bool, error)
  func (s *sqlStore) markExhausted(ctx context.Context, fence LeaseFence, intentID string, exhaustedReason string, now time.Time) (bool, error)
  func (s *sqlStore) recordWebhookAttempt(ctx context.Context, fence LeaseFence, intentID string, status string, attemptedAt time.Time, errMsg string) (bool, error)

Define `LeaseFence` in `submissionmanager` with holder_id and lease_epoch used to guard writes via `WHERE EXISTS (...)` predicates against `submission_manager_leases` and `expires_at > SYSUTCDATETIME()`.

All SQL comparisons that decide lease validity or intent due-ness must use `SYSUTCDATETIME()` within the SQL statements, not application time parameters.

---

Change log: execplan created from `specs/submission-manager-leaderlease.md` to drive leader-lease implementation. (2026-02-03 / Codex)

Plan update: Removed spec document edits from the plan and limited documentation changes to `backend/README.md` to comply with EXEC restrictions on `specs/`. (2026-02-03 / Codex)

Plan update: Marked progress complete after implementing leader lease, fencing, readiness output, tests, and running `go test ./...`. (2026-02-03 / Codex)
