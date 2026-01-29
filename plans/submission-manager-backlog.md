# SubmissionManager backlog (post-Phase 2)

This list captures follow-up ideas and open questions to consider in later phases. It is not an execution plan.

- Persistence and recovery: durable intent store, idempotency across restarts, retention/cleanup policy.
- HTTP APIs: submit/get intent endpoints, error surface, and versioning expectations.
- Retry policies: configurable strategies (fixed, exponential, jitter) and per-target execution overrides.
- Attempt timeouts: gateway-request timeout alignment and handling of late responses.
- Concurrency model: worker scaling, queue fairness, and deterministic scheduling under load.
- Cancellation: intent cancelation semantics, in-flight attempt handling, and client-facing outcomes.
- Metrics/observability: per-intent lifecycle metrics, retry counts, policy exhaustion reasons.
- Failure taxonomy: explicit classification for executor errors vs gateway outcomes.
- Contract evolution: how to handle contract changes after intent creation (versioning, snapshot retention).
- Routing evolution: registry reload strategy, validation, and impact on running managers.
- Delivery tracking: separate system integration once acceptance is not the end state.
- Multi-tenant isolation: per-tenant limits and policy overrides, if needed.
- Testing strategy: broader end-to-end tests once HTTP surface exists.
