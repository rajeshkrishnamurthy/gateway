# AGENTS.md - submission

## Scope
This folder owns the SubmissionTarget registry and contract validation used by SubmissionManager.

## Boundaries
- No SubmissionManager scheduling or attempt execution here.
- No net/http usage.
- No gateway behavior changes; only validate known gateway outcomes.
- Keep code explicit and flat; stdlib only.

## Contracts
- submissionTarget is data-driven and bound explicitly to gatewayType in the registry.
- gatewayType is code-known and defines protocol + response semantics.
- policy selects the retry termination rule (deadline, max_attempts, one_shot).
- terminalOutcomes are gateway-reported outcomes treated as terminal by the contract.
- maxAcceptanceSeconds is a cumulative wall-clock bound across all attempts when policy is `deadline`.
- maxAttempts is required when policy is `max_attempts`.
