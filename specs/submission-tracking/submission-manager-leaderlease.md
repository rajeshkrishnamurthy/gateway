# SubmissionManager leader lease (SQL Server)
COMPLETED

## Purpose

Enable multiple SubmissionManager instances behind HAProxy for high-availability HTTP while guaranteeing exactly one executor at any time, using SQL Server as the source of truth.

## Scope

- Multiple SubmissionManager instances accept HTTP requests behind HAProxy.
- Exactly one leader executes attempts using a SQL-backed lease.
- The leader builds and maintains the in-memory schedule from SQL persistence.
- Followers do not execute attempts and do not build schedules.
- SQL Server is the source of truth for leadership and scheduling state.

## Non-goals

- No per-intent claiming or multi-worker execution.
- No gateway contract changes.
- No delivery tracking or callbacks.

## Summary

- All instances serve the HTTP API.
- Exactly one instance (leader) runs the executor loop.
- Leadership is represented by a single SQL lease row with an expiry time based on SQL Server time.
- The leader incrementally refreshes its in-memory schedule from SQL using a composite cursor on intent `last_modified_at`.
- Loss of lease stops further attempt execution immediately.

## Concepts

### Lease

A single SQL row represents the leadership lease. The lease has:

- lease_name (constant name for the executor lease)
- holder_id (instance identity)
- expires_at (UTC timestamp in SQL Server time)
- acquired_at, renewed_at (UTC, SQL Server time)
- lease_epoch (monotonic integer, increments on successful acquire)

### Leader vs follower

- Leader: holds the lease and runs the executor loop.
- Follower: serves HTTP only; must not execute attempts.

## Data model

Table: submission_manager_leases

- lease_name varchar(64) PK
- holder_id varchar(128) NOT NULL
- lease_epoch bigint NOT NULL
- acquired_at datetime2(7) NOT NULL
- renewed_at datetime2(7) NOT NULL
- expires_at datetime2(7) NOT NULL

Notes:

- All timestamps use SQL Server time (SYSUTCDATETIME()).
- Only one row is expected for lease_name = 'submission-manager-executor'.

### Intent scheduling watermark (submission_intents)

The `submission_intents` table requires a monotonic update marker so the leader can incrementally refresh its schedule.

- last_modified_at datetime2(7) NOT NULL

Rules:

- `last_modified_at` is set on insert and updated on **every** update to the intent row.
- Updates use SQL Server time (SYSUTCDATETIME()) in the same statement or transaction as the intent write.

## Lease operations

### Acquire (or re-acquire)

Acquire succeeds if no row exists or if the current lease has expired.

Atomic behavior:

- If row exists and expires_at > now, acquire fails.
- If row exists and expired, update it to the current holder and extend expiry.
- If row is missing, insert it.

On success:

- Set acquired_at and renewed_at to now.
- Set expires_at to now + lease_duration.
- Increment lease_epoch on every successful acquire, even if the same holder_id re-acquires after expiry (initialize to 1 if inserting). Lease_epoch does not change on renew.

### Renew

Leader renews periodically by extending expires_at only if the row is still held by the same holder_id and the lease is not expired.

If renew fails (row not held, lease expired, SQL error, or timeout), leadership is lost immediately.

Explicit release on shutdown is omitted in this phase.

## Invariants

- Exactly one leader executes attempts at any time; followers must not execute attempts.
- All execution state mutations are fenced by the current lease_epoch (see below).
- All lease and schedule time comparisons use SQL Server time.
- Only the leader builds and refreshes the in-memory schedule.

### Fencing invariant (mandatory)

lease_epoch is a fencing token. Every executor-side write that mutates execution state MUST be conditional on holding the current lease_epoch at the time of the write.

Acceptable enforcement patterns:

- Include lease_epoch (and holder_id) in every UPDATE/INSERT predicate, or
- Re-read (holder_id, lease_epoch) immediately before each state transition and fence the write with that value.

If a fenced write affects 0 rows, the executor MUST treat it as lease loss, stop execution, and drop leadership.

### Time base

All lease comparisons use SYSUTCDATETIME() inside SQL statements. No comparisons use local machine time.

## Concurrency guarantees

- Only the leader may dequeue and begin new attempts.
- The leader must re-check leadership before dequeueing and before executing each attempt.
- All executor-side writes that mutate execution state are fenced by (holder_id, lease_epoch).
- Followers may accept HTTP writes, but they must not execute attempts.
- Schedule refresh must be idempotent and must not enqueue duplicate attempts when the same intent row is observed more than once.

## Runtime behavior

### Startup

1. Start as follower.
2. Attempt to acquire the lease.
3. If acquired, become leader, rebuild the schedule from SQL, and start the executor loop.
4. If not acquired, remain follower and retry acquisition periodically.

### Renewal loop

- Run every renew_interval.
- On success: keep leadership.
- On failure or SQL error: drop leadership and stop executor.

### Executor loop gating

Leader-only:

- Before dequeueing an attempt.
- Before executing an attempt.

If leadership is lost:

- No new attempts may begin.
- In-flight attempt handling depends on whether provider submission has occurred:
  - If provider submission has not yet happened, abort the attempt.
  - If provider submission has happened, the attempt may complete, but no further state transitions may occur without a leadership check and fencing.

### Leadership transitions

- Leader -> Follower on lease loss or renew failure.
- Follower -> Leader on successful acquire.

### Schedule rebuild

Only the leader builds the in-memory schedule. Followers do not build schedules until they become leader.
On leadership acquisition, the leader performs a full rebuild from SQL persistence and establishes the initial refresh cursor.
This reduces follower load at the cost of a short rebuild delay during failover.

### Schedule refresh (leader-only)

The leader incrementally refreshes its in-memory schedule from SQL so it can pick up intents submitted to followers.

Refresh loop:

- Run every `schedule_refresh_interval`.
- Query using the composite cursor `(last_modified_at, intent_id)`:
  - `WHERE (last_modified_at > @t) OR (last_modified_at = @t AND intent_id > @id)`
  - `ORDER BY last_modified_at, intent_id`
- Load any changed intents into the in-memory schedule (including newly created intents and reschedules).
- Advance the cursor to the last row processed.

Rules:

- `last_modified_at` is updated on every intent insert and update using SQL Server time.
- If the refresh query fails with a SQL error, treat it as a SQL connectivity failure (drop leadership and stop the executor).

## Race-condition handling

### Concurrent acquire attempts

Multiple followers may try to acquire at the same time when the lease expires.
Handling: acquire is a single conditional insert/update using SQL Server time; only one statement succeeds and others observe zero rows affected.

### Acquire vs renew overlap

A follower may try to acquire while the leader renews.
Handling: renew requires holder_id match and expires_at > now, while acquire requires expires_at <= now. These predicates are mutually exclusive when evaluated in SQL Server time, so only one succeeds. If renew wins, acquire fails. If acquire wins after expiry, renew fails and the old leader drops leadership.

### Expiry boundary race

Multiple instances may observe the lease expiring at nearly the same time.
Handling: the acquire statement uses a single SQL predicate on expires_at <= now, so only one update or insert succeeds.

### Leader pause or long GC

If the leader stops renewing, the lease expires and another instance may become leader.
Handling: the executor must stop on renew failure and must check leadership before dequeueing and before executing each attempt. Any in-flight attempt may complete, but no new attempt may start.

### SQL connectivity loss (split-brain risk)

If the leader cannot reach SQL, it cannot prove it still holds the lease.
Handling: any renew failure or SQL error immediately drops leadership and stops the executor.

### Leadership loss during schedule rebuild

Leadership can be lost while rebuilding the in-memory schedule.
Handling: after rebuild and before executing the first attempt, re-check leadership; if lost, discard the schedule and stop.

### Schedule refresh vs concurrent intent updates

An intent may be inserted or updated while the leader is refreshing its schedule.
Handling: the composite cursor `(last_modified_at, intent_id)` with ordered pagination prevents gaps; any update after the current cursor is observed on a later refresh. Duplicate observations are safe because refresh is idempotent.

### Likelihood and impact

This race is low likelihood per event but high likelihood over long runtimes. DB/network stalls or long pauses can exceed the lease duration, leading to lease expiry and concurrent execution.
Impact is high: split-brain execution can cause conflicting state writes and duplicate external submissions. The fencing invariant above is mandatory to prevent conflicting state writes even when overlapping execution occurs.

## Health and diagnostics

### Logs

Emit structured logs on:

- leader_acquired
- leader_renewed
- leader_lost
- leader_acquire_failed
- leader_renew_failed

Include: holder_id, lease_epoch, expires_at, sql_error (when present).

### Health endpoints

- /healthz unchanged (HTTP availability).
- /readyz includes role info.

Example response body:

mode=leader holder_id=sm-01 lease_expires_at=2026-02-02T12:00:10Z

(Exact format is not a client contract; for humans/operators.)

/readyz returns 200 for both leaders and followers so followers remain in HTTP rotation.

## Configuration (internal)

- lease_duration (default 60s)
- renew_interval (default 20s; must be < lease_duration)
- acquire_interval (default 30s)
- schedule_refresh_interval (default 1s)
- holder_id (default: hostname-pid-rand)
- lease_name (default: submission-manager-executor)

## Failure semantics

### Concurrent startup

Only one instance acquires the lease because the acquire/renew update is conditional.

### Leader crash

Lease expires after lease_duration. A follower acquires and becomes leader.

### Leader restart

If the old lease is expired, the instance can re-acquire; otherwise it stays follower until expiry.

### SQL connectivity issues

On renewal failure or any SQL error (including schedule refresh):

- Leadership is dropped immediately.
- Executor loop stops.
- Instance stays follower until it can reacquire.

## Compatibility with existing semantics

- Contract snapshot per intent is unchanged.
- Attempts remain append-only; attempt_count is authoritative.
- Idempotency remains across restarts.
- Scheduling is still rebuilt from SQL on leader startup.
- HTTP behavior and gateway interaction are unchanged.

## Observable acceptance criteria

- With concurrent startup, exactly one leader is elected and only that leader executes attempts.
- On lease expiry or crash, a follower becomes leader and begins execution after acquiring the lease.
- The leader never executes attempts without holding the lease; leadership loss stops new attempts immediately.
- With `schedule_refresh_interval` configured and SQL healthy, a new intent inserted by a follower is observed and enqueued by the leader no later than the next refresh cycle.
- Incremental refresh uses the composite cursor `(last_modified_at, intent_id)` and does not skip intents that share the same `last_modified_at`.

## Tests (must exist)

1. Single leader on concurrent start.
2. Failover after expiry.
3. No execution without leadership.
4. Leader stops executing immediately on lease loss.
5. Leader picks up follower-submitted intents via schedule refresh.
