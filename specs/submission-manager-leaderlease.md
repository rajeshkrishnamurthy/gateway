 ———

  Solution (short)
  Use a single SQL‑Server–backed lease row to elect exactly one leader. Every instance runs the HTTP API; only the current lease holder runs the executor
  loop. Leadership is renewed on a timer; loss of lease immediately stops further attempt execution. All lease comparisons use SQL Server time to avoid clock
  skew.

  ———

  # Spec draft — SubmissionManager leadership lease (SQL Server)

  ## Purpose

  Enable multiple SubmissionManager instances behind HAProxy for high‑availability HTTP while guaranteeing exactly one executor at any time, using SQL Server
  as the source of truth.

  ## Non‑goals

  - No per‑intent claiming or multi‑worker execution.
  - No gateway contract changes.
  - No delivery tracking or callbacks.

  ## New concepts

  ### Lease

  A single SQL row represents the leadership lease. The lease has:

  - lease_name (constant name for the executor lease)
  - holder_id (instance identity)
  - expires_at (UTC timestamp in SQL Server time)
  - acquired_at, renewed_at (UTC, SQL time)
  - lease_epoch (monotonic integer, increments on successful acquire; used for diagnostics)

  ### Leader vs Follower

  - Leader: holds the lease and runs the executor loop.
  - Follower: serves HTTP only; must not execute attempts.

  ## SQL schema

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

  ## Lease operations

  ### Acquire (or re‑acquire)

  Acquire succeeds if no row exists or if the current lease has expired.

  Atomic behavior:

  - If row exists and expires_at > now, acquire fails.
  - If row exists and expired, update it to current holder and extend expiry.
  - If row missing, insert it.

  Lease epoch

  - Increment lease_epoch on successful acquire (not on renew).

  ### Renew

  Leader renews periodically by extending expires_at only if the row is still held by the same holder_id.

  If renew fails (row not held, SQL error, or timeout), leadership is lost immediately.

  ### Time base

  All lease comparisons use SYSUTCDATETIME() inside SQL statements.
  No comparisons use local machine time.

  ## Runtime behavior

  ### Startup

  1. Start as follower.
  2. Attempt to acquire lease.
  3. If acquired, become leader and start executor loop.
  4. If not acquired, remain follower and retry acquisition periodically.

  ### Renewal loop

  - Run every renew_interval.
  - On success: keep leadership.
  - On failure or SQL error: drop leadership and stop executor.

  ### Executor loop gating

  Leader‑only:

  - Before dequeueing an attempt.
  - Before executing an attempt.

  If leadership is lost:

  - No new attempts may begin.
  - Any in‑flight attempt may complete (single‑threaded loop), then execution stops.

  ### Leadership transitions

  - Leader → Follower on lease loss or renew failure.
  - Follower → Leader on successful acquire.

  ## Health & diagnostics

  ### Logs

  Emit structured logs on:

  - leader_acquired
  - leader_renewed
  - leader_lost
  - leader_acquire_failed
  - leader_renew_failed

  Include: holder_id, lease_epoch, expires_at, sql_error if any.

  ### Health endpoints

  - /healthz unchanged (HTTP availability).
  - /readyz includes role info.

  Required behavior: /readyz response must state leader or follower mode for diagnostics.
  Example response body:

  mode=leader holder_id=sm-01 lease_expires_at=2026-02-02T12:00:10Z

  (Exact format is not a client contract; for human/operators.)

  ## Configuration

  Minimal internal knobs (not part of client contract):

  - lease_duration (default 20s)
  - renew_interval (default 5s; must be < lease_duration)
  - holder_id (default: hostname-pid-rand)

  Optional:

  - lease_name (default: submission-manager-executor)

  ## Failure handling

  ### Concurrent startup

 plans/admin-portal-troubleshoot-execplan.md                      |  212 ++++++++++++++++
 plans/intent-history-execplan.md                                 |  186 ++++++++++++++
 ui/health_overview.tmpl                                          |    4 +-
 ui/health_services.tmpl                                          |   47 ----
 ui/manager_history_results.tmpl                                  |   74 ++++++
  Only one instance will acquire the lease because the acquire/renew update is conditional.

  ### Leader crash

  Lease expires after lease_duration. Next follower acquires and becomes leader.

  ### Leader restart

  If old lease expired, it can re‑acquire; otherwise it stays follower until expiry.

  ### SQL connectivity issues

  On renewal failure or SQL error:

  - leadership is dropped immediately.
  - executor loop stops.
  - instance stays follower until it can reacquire.

  ## Compatibility with existing semantics

  - Contract snapshot per intent is unchanged.
  - Attempts remain append‑only; attempt_count is authoritative.
  - Idempotency remains across restarts.
  - Scheduling is still rebuilt from SQL on leader startup.
  - HTTP behavior and gateway interaction are unchanged.

  ## Tests (must exist)

  1. Single leader on concurrent start
     Start two instances; assert only one becomes leader and executes attempts.
  2. Failover after expiry
     Kill leader (or stop renew). Verify follower acquires after lease expiry.
  3. No execution without leadership
     Ensure attempts are not executed by followers (even if scheduled).

