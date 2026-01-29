# submissionmanager

This package implements the in-memory SubmissionManager execution engine (Phase 2).

Key points:

- SubmissionManager resolves submissionTarget via the SubmissionTarget registry and stores a contract snapshot on each intent.
- It executes attempts via a provided AttemptExecutor and manages intent state transitions (accepted, rejected, exhausted).
- Retry timing uses a fixed 5 second delay as an internal execution policy, not a contract term.
- The engine is HTTP-agnostic and has no persistence; restarting clears state.

See `backend/spec/submission-manager.md` for domain semantics and `backend/submission/README.md` for registry rules.
