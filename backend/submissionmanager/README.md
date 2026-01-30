# submissionmanager

This package implements the SubmissionManager execution engine with SQL Server durability (Phase 3a).

Key points:

- SubmissionManager resolves submissionTarget via the SubmissionTarget registry and stores a contract snapshot on each intent.
- It executes attempts via a provided AttemptExecutor and manages intent state transitions (accepted, rejected, exhausted).
- Retry timing uses a fixed 5 second delay as an internal execution policy, not a contract term.
- Intent state (including attempts and nextAttemptAt) is persisted in SQL Server; the in-memory queue is rebuilt on startup.
- SQL schema lives in `backend/conf/sql/submissionmanager`.

See `backend/spec/submission-manager.md` for domain semantics and `backend/submission/README.md` for registry rules.
