# AGENTS.md - submissionmanager

## Scope
This folder owns the SubmissionManager execution engine and its durable state.

## Boundaries
- No net/http usage.
- Do not change gateway behavior or outcome taxonomies.
- Use the SubmissionTarget registry in `backend/submission` for contract resolution.
- Retry timing is an internal execution detail, not a contract term.
- SQL Server is the durable store for intents, attempts, and scheduling metadata.
