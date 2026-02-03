# submission

This package defines the SubmissionTarget registry and contract validation used by SubmissionManager.

Key points:

- HTTP-agnostic core logic only.
- SubmissionTarget is data-driven and binds explicitly to a gatewayType.
- gatewayType is code-known and defines protocol + response semantics.
- policy selects the retry termination rule (deadline, max_attempts, one_shot).
- terminalOutcomes are gateway-reported outcomes treated as terminal by the contract.
- maxAcceptanceSeconds is a cumulative wall-clock bound across all attempts when policy is `deadline`.
- maxAttempts is required when policy is `max_attempts`.
- webhook config is optional and lives on the submissionTarget contract; unsigned webhooks require explicit allowUnsignedWebhooks in the registry file.

Use `LoadRegistry(path)` to load and validate the registry, and `Registry.ContractFor(target)` to look up a contract.

See `specs/submission-manager.md` for the formal contract definitions. The sample registry config lives at `backend/conf/submission/submission_targets.json`.
