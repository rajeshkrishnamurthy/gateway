# AGENTS.md - submissionmanager

## Scope
This folder owns the in-memory SubmissionManager execution engine.

## Boundaries
- No net/http usage.
- No persistence or durable storage.
- Do not change gateway behavior or outcome taxonomies.
- Use the SubmissionTarget registry in `backend/submission` for contract resolution.
- Retry timing is an internal execution detail, not a contract term.
