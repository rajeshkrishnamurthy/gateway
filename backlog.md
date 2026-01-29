# Backlog

This list captures follow-up ideas and open questions to consider in later phases. It is not an execution plan.

- Persistence and recovery: Decide if intents must survive restarts, how to recover in-flight attempts, and what retention/cleanup policy looks like. Code to examine: `backend/submissionmanager/manager.go`, `backend/submissionmanager/manager_test.go`.
- Idempotency scope and retention: Choose whether idempotency spans all historical intents or only active ones, and whether a separate idempotency ledger/TTL is needed. Code to examine: `backend/submissionmanager/manager.go`, `backend/submissionmanager/manager_test.go`.
- HTTP APIs: Define submit/get intent endpoints, error surfaces, and versioning so clients can rely on stable semantics. Code to examine: `backend/cmd/sms-gateway/main.go`, `backend/cmd/push-gateway/main.go`, and `backend/cmd/admin-portal/main.go` for handler patterns.
- Retry policies: Move from a fixed delay to configurable strategies (fixed, exponential, jitter) without exposing timing as a contract term. Code to examine: `backend/submissionmanager/manager.go`.
- Attempt timeouts: Decide how long the manager waits for a gateway response and how late responses are handled; align with gateway server timeouts. Code to examine: `backend/submissionmanager/manager.go`, `backend/cmd/sms-gateway/main.go`, `backend/cmd/push-gateway/main.go`.
- Concurrency model: Consider worker scaling, queue fairness, and deterministic scheduling under load. Code to examine: `backend/submissionmanager/manager.go`.
- Cancellation: Define how cancellation affects in-flight attempts and which final status a client sees. Code to examine: `backend/submissionmanager/manager.go`.
- Metrics/observability: Add per-intent lifecycle metrics, retry counts, and exhaustion reasons for operators. Code to examine: `backend/metrics/metrics.go`, `backend/submissionmanager/manager.go`.
- Failure taxonomy: Make executor errors distinct from gateway outcomes and record how they affect policy decisions. Code to examine: `backend/submissionmanager/manager.go`, `backend/submission/registry.go`.
- Contract evolution: Decide how to handle registry changes after intent creation (versioning, migrations, backward compatibility). Code to examine: `backend/submission/registry.go`, `backend/submissionmanager/manager.go`.
- Routing evolution: Define registry reload behavior and its effect on in-flight intents while preserving contract snapshots. Code to examine: `backend/submission/registry.go`, `backend/submissionmanager/manager.go`.
- Delivery tracking: Plan integration with a separate delivery-status system once acceptance is not the end state. Code to examine: none in this repo yet (new module expected).
- Multi-tenant isolation: Decide per-tenant limits and policy overrides, ensuring tenant identifiers flow consistently. Code to examine: `backend/sms_gateway.go`, `backend/push_gateway.go`, `backend/submissionmanager/manager.go`.
- Testing strategy: Add end-to-end coverage once the HTTP surface exists, beyond unit tests. Code to examine: `backend/submissionmanager/manager_test.go`.
