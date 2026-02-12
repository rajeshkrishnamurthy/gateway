# Submission Tracking Ubiquitous Language
EXEC-READY

## Purpose

This glossary is the canonical terminology source for the submission-tracking domain. Submission-tracking specs and related execution docs must use these terms consistently and must not redefine them implicitly.

## Terms

### SubmissionIntent

A SubmissionIntent is the client-facing unit of work created by `POST /v1/intents`. It represents one intent lifecycle and may contain multiple attempts.

### intentId

`intentId` is the stable idempotency key for a SubmissionIntent. Reusing an `intentId` with identical `submissionTarget` and payload is idempotent; reusing it with different values is an idempotency conflict.

### submissionTarget

`submissionTarget` is the data-driven contract identifier supplied by the client. It selects contract semantics and resolves to a bound `gatewayType` and `gatewayUrl` through the registry.

### gatewayType

`gatewayType` is a code-known gateway family (`sms` or `push`) that defines gateway response taxonomy and protocol semantics. It is not supplied directly by the client.

### Contract snapshot

The contract snapshot is the resolved contract data persisted with each SubmissionIntent at submit time. It includes routing and policy data used for deterministic execution across retries and restarts.

### Attempt

An Attempt is one execution try for a SubmissionIntent. Attempts are append-only audit records and are ordered by attempt number.

### Intent status

Intent status is the orchestration status for a SubmissionIntent and is one of `pending`, `accepted`, `rejected`, or `exhausted`.

### Terminal status

A terminal status is one of `accepted`, `rejected`, or `exhausted`. Terminal intents are immutable and must not transition again.

### Rejected reason

Rejected reason is the terminal gateway outcome reason recorded only when intent status is `rejected`.

### Exhausted reason

Exhausted reason is the policy termination reason recorded only when intent status is `exhausted`. It describes policy exhaustion, not gateway rejection.

### waitSeconds

`waitSeconds` is the optional `POST /v1/intents` query parameter that controls synchronous response wait duration. It is transport-only behavior and is not part of intent identity or persisted contract data.

### Intent history

Intent history is the retrieval shape that returns the current intent view and its ordered attempts for one `intentId`.

### SubmissionManager HTTP boundary

The SubmissionManager HTTP boundary owns submission orchestration routes (`/v1/intents*`), health/readiness/metrics routes, and the optional history UI fragment route. It must not own delivery-tracking read routes or provider delivery webhook ingress.
