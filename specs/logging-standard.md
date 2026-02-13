# Setu Logging Standard

EXEC-READY

## Purpose / Big Picture

Setu reliability depends on fast, evidence-based troubleshooting across three communication profiles:

- outside systems and Setu boundaries
- Setu service to Setu service boundaries
- decisions within a Setu service

Logging is therefore a first-class contract, not auxiliary output. This standard defines what must be logged for each profile, how logs must be emitted, and how Setu must parse those logs for operational evidence and proactive detection.

## Scope

This standard governs:

- runtime server logs emitted by Setu backend services and adapters
- implementation conventions for Go logging in Setu services
- machine-parseable log format requirements for production
- profile-specific required fields and event semantics
- Setu-owned parser/normalizer behavior for operational tooling

This standard is mandatory for all new or modified Setu backend logging behavior.

## Non-goals

This standard does not define:

- metrics naming or dashboard policy
- distributed tracing span schema
- external log-storage vendor choice, retention policy, or SIEM configuration
- browser/client-side UI logging

## Normative Layering

This document defines the cross-cutting baseline. Domain specs may add stricter requirements but must not weaken this contract.

Relevant domain specs include:

- `specs/gateway-contracts.md`
- `specs/model-provider-adapter.md`
- `specs/submission-tracking/submission-manager-leaderlease.md`
- `specs/delivery-tracking/delivery-tracking-v1-observability.md`

## Implementation Method (Go)

Setu backend services must use structured logging for decision and evidence events. The default Setu implementation method for Go must be the standard library `log/slog`.

New or modified backend code must emit decision/evidence logs through `slog` logger methods (directly or through a thin wrapper that preserves this standard). Raw `log.Printf` must not be used for new or modified decision/evidence log statements.

Production runtime logging must use a JSON handler and emit one JSON object per line. Local developer runs may use a text handler, but event names, key names, and value vocabularies must remain equivalent to the production schema.

## Communication Profiles (Mandatory)

Every decision/evidence record must declare exactly one `commProfile` from this fixed set:

- `outside_to_setu`
- `setu_to_setu`
- `within_setu_service`

Profile meanings:

- `outside_to_setu`: boundary communication between Setu and external systems (client ingress, provider webhook ingress, or Setu egress to external provider/API).
- `setu_to_setu`: boundary communication between two Setu services.
- `within_setu_service`: decision/state processing entirely within one Setu service.

Profile vocabulary is bounded. New profile values require a SPEC change.

## Machine-Parseable Record Contract

Production logs must be newline-delimited JSON (one event per line), UTF-8 encoded, and machine-parseable without regex interpretation of free-form prose.

Each decision/evidence record must include these base fields:

- `logSchemaVersion` (schema identifier/version, for example `setu.log.v1`)
- `time`
- `level`
- `msg`
- `event`
- `service`
- `component`
- `instance`
- `commProfile`

Correlation fields must be included whenever available and applicable:

- `intentId`
- `referenceId`
- `attempt`
- `submissionTarget`
- `gatewayType`
- `gatewayMessageId`
- `provider`
- `sourceRecordId`
- `deliveryTrackingMode`
- `correlationResult`
- `leaseEpoch`
- `traceId`
- `spanId`

Profile-specific required fields:

- For `outside_to_setu` boundary completion/rejection events:
  - `boundaryDirection` (`ingress` or `egress`)
  - `peerSystem`
  - `operation`
  - `outcome`
- For `setu_to_setu` call completion/failure events:
  - `callerService`
  - `calleeService`
  - `operation`
  - `outcome`
  - `durationMs`
- For `within_setu_service` decision/transition events:
  - `operation`
  - `stage`
  - `outcome`

Field keys must remain stable. Renaming or repurposing an existing key requires a SPEC change and schema-version update rules.

## Decision and Evidence Event Contract

Logs must provide one attributable evidence trail for each externally significant boundary decision and for each internal state decision that affects outcomes.

Minimum event families per communication profile:

- `outside_to_setu` must include boundary outcome evidence for:
  - request validation/rejection and normalized response outcomes for client-facing endpoints
  - provider webhook ingress outcomes
  - outbound provider/external API call attempt and mapped outcome
- `setu_to_setu` must include boundary outcome evidence for:
  - internal request/call start and completion
  - timeout/failure classification
  - retry-attempt attribution when retries are part of contract behavior
- `within_setu_service` must include outcome evidence for:
  - lifecycle decisions (for example attempt start/result/retry/terminal)
  - domain state transitions and no-op reasons
  - lease/leadership state transitions and processing-stage failures

Setu must continue to emit existing required domain events, including:

- gateway request validation and normalized final response outcomes
- provider request attempts, provider response classification, and provider-to-gateway mapping decisions
- SubmissionManager attempt lifecycle outcomes (`start`, `result`, `retry`, `terminal`)
- delivery-tracking ingestion rejection, correlation outcomes, ignored-signal audit outcomes (`mode_off` when applicable), state transitions, and processing failures
- leader lease transitions (`leader_acquired`, `leader_renewed`, `leader_lost`, `leader_acquire_failed`, `leader_renew_failed`)

Decision reasons must use bounded vocabularies defined by canonical specs. Free-form reason values are not allowed for contract outcomes.

## Parser and Normalizer Contract

Setu must own machine parsing as part of the product, not as ad hoc external scripts.

Setu must include in-repo parser/normalizer code that:

- ingests production log JSON lines and validates schema conformance
- validates `commProfile` and profile-specific required fields
- maps records into typed normalized events for troubleshooting/evidence workflows
- supports the current `logSchemaVersion` and the immediately previous version during migrations
- handles malformed records safely (record-level failure reporting without crashing whole stream processing)

Parser behavior must be covered by tests with positive fixtures and negative fixtures (missing required fields, invalid enum values, malformed JSON, and redaction violations), including profile-specific fixture coverage for all three `commProfile` values.

Operational tooling that depends on Setu logs for troubleshooting evidence must use the Setu parser/normalizer, not custom regex-only parsing.

## Data Safety Contract

Logs must not expose secrets or sensitive content.

The following must not be logged:

- credentials, auth headers, tokens, API keys, or secret config values
- full message content or raw payload bodies containing client content
- full recipient/token identifiers when masking/redaction is required

When sensitive information is needed for troubleshooting, logs must use safe derivatives only, such as:

- masked recipient/token values
- `messageLen`
- `messageHash`
- explicitly gated truncated/sanitized provider error summaries

These safety requirements apply uniformly to all three communication profiles.

## Invariants

- Logging must preserve deterministic attribution for client-facing, inter-service, and internal decision outcomes.
- Logs must be sufficient to reconstruct why Setu produced a specific normalized outcome.
- Every decision/evidence event must be classifiable into exactly one communication profile.
- Logging and parser behavior must not alter domain semantics (`submission status`, `delivery status`, `delivery freshness`, retry outcomes).
- Logging schema and event vocabularies must remain queryable and stable under multi-instance operation.
- Required mode-off audit visibility (`mode_off`) must remain present where delivery-tracking specs require it.

## Race Conditions and Handling

Concurrent requests, retries, and provider signals may interleave across instances. Logs and parser output must remain attributable under this interleaving.

- Operations sharing identifiers must remain distinguishable using fields such as `attempt`, `event`, `instance`, and profile-specific boundary fields.
- Multi-instance records may arrive out of order; reconstruction must rely on correlation keys and timestamps, not ingestion order alone.
- Duplicate/out-of-order delivery signals must produce non-contradictory no-op/transition evidence.
- Inter-service retries and timeout races must remain diagnosable through stable caller/callee/attempt attribution.
- Lease failover races must remain diagnosable through stable lease-attribution fields.

## Failure Semantics

- Logging and parsing are required capabilities but must be operationally non-blocking to domain outcomes.
- Logging emission failures must degrade observability only and must not change response normalization or state transitions.
- Parser failures must degrade evidence workflows only and must not affect runtime request processing behavior.
- Redaction/sanitization failures must fail safe and must not emit raw sensitive values.
- Missing optional tracing context (`traceId`/`spanId`) must not block emission of required logging evidence.

## Concurrency Guarantees

- For each completed boundary operation, decision evidence must carry a stable correlation key set and the correct `commProfile`.
- Terminal decision evidence must be emitted exactly once per component boundary that owns that decision.
- Logging and parser code must be safe under concurrent goroutines and multi-instance deployment.
- Equivalent outcomes across instances must use equivalent event names and key schemas.
- Equivalent outcomes across communication profiles must remain distinguishable by profile-specific fields.

## Observable Acceptance Criteria

Criterion 1: production logs are newline-delimited JSON records and each decision/evidence record includes required base fields (`logSchemaVersion`, `time`, `level`, `msg`, `event`, `service`, `component`, `instance`, `commProfile`).

Criterion 2: new or modified backend decision/evidence logs are emitted via `log/slog` APIs (or a compliant thin wrapper), not raw `log.Printf`.

Criterion 3: `commProfile` uses only allowed values (`outside_to_setu`, `setu_to_setu`, `within_setu_service`) and profile-specific required fields are present for representative events.

Criterion 4: gateway flows emit `outside_to_setu` decision evidence with normalized outcome attribution and bounded reason/source values when applicable.

Criterion 5: provider adapter external-call evidence is emitted as `outside_to_setu` with `boundaryDirection="egress"` and sanitized mapping outcomes.

Criterion 6: internal Setu service calls emit `setu_to_setu` completion/failure evidence with caller/callee attribution and duration.

Criterion 7: SubmissionManager lifecycle and leader lease evidence is emitted as `within_setu_service` with required attempt/lease attribution.

Criterion 8: delivery-tracking emits required rejection, correlation (`unmatched`/`invalid`), `mode_off`, transition, and processing-failure evidence events with appropriate profile attribution.

Criterion 9: forbidden sensitive content classes are absent from logs in representative runs across all three communication profiles.

Criterion 10: Setu repository includes parser/normalizer code and automated tests covering schema-valid, malformed, redaction-sensitive, and profile-specific fixtures.

Criterion 11: parser outputs remain compatible across current and previous log schema versions during planned schema migrations.

Criterion 12: logging backend or parser-tooling failures do not change Setu domain behavior for submission or delivery processing.
