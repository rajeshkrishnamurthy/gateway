# SubmissionManager Swagger Compatibility
EXEC-READY

## Purpose / Big picture

SubmissionManager must publish a Swagger-compatible API contract so operators, integrators, and downstream tooling can consume a complete and accurate HTTP surface for this bounded context. This compatibility contract must improve API discoverability and validation without changing submission-tracking semantics defined in existing specs.

This spec defines what must be true for SubmissionManager to be considered fully Swagger compatible. It is strictly a contract-coverage requirement for owned HTTP behavior, not a retry, policy, leader-lease, or gateway-semantics change.

## Scope

This spec applies to the SubmissionManager bounded context HTTP boundary and must cover all owned routes and methods:

- `GET /healthz`
- `GET /readyz`
- `GET /metrics`
- `POST /v1/intents`
- `GET /v1/intents/{intentId}`
- `GET /v1/intents/{intentId}/history`
- `POST /ui/history` (when UI templates are enabled)

The Swagger-compatible contract must describe request parameters, request bodies, response status codes, response media types, and response schemas for these routes. For JSON routes, the contract must include the canonical intent and error payload shapes already defined by submission-tracking behavior. For non-JSON routes, the contract must declare the correct media type (`text/plain` or `text/html`) and method constraints.

This spec extends, but does not replace, existing submission-tracking semantics in:

- `specs/submission-tracking/submission-manager.md`
- `specs/manager-sync-timeout.md`
- `specs/submission-tracking/submission-manager-webhooks.md`
- `specs/submission-tracking/submission-manager-leaderlease.md`
- `specs/submission-tracking/submission-manager-metrics.md`

Terminology must follow `specs/submission-tracking/ubiquitous-language.md`.

## Non-goals

This work must not change execution behavior, policy behavior, idempotency behavior, lease behavior, or gateway behavior. It must not introduce delivery-tracking ownership into SubmissionManager and must not move delivery routes into this bounded context.

This work must not redefine existing status semantics (`pending`, `accepted`, `rejected`, `exhausted`), wait semantics (`waitSeconds`), or webhook semantics. It must not introduce new client-visible business fields solely for documentation convenience.

## Invariants (explicit)

SubmissionManager Swagger compatibility must satisfy all invariants below.

1. A machine-readable OpenAPI document compatible with Swagger tooling is required and must be the canonical HTTP contract artifact for SubmissionManager. The canonical file path is `specs/submission-tracking/submission-manager-openapi.yaml`.
2. The OpenAPI document must use OpenAPI 3.0.x syntax, must pass structural validation by standard Swagger/OpenAPI validators, and must render in Swagger UI without parser errors.
3. The documented operation set must exactly match SubmissionManager-owned path+method pairs in scope; undocumented owned operations are not allowed and extra non-owned operations are not allowed.
4. JSON response contracts must preserve current payload semantics:
   - `intentResponse` fields and status values remain unchanged.
   - `intentHistoryResponse` shape remains unchanged.
   - Error payloads remain `{ "error": { "code", "message", "details?" } }`.
5. `waitSeconds` remains transport-only and must not become part of idempotency identity or persisted intent state.
6. SubmissionManager non-ownership boundaries remain explicit:
   - `POST /v1/delivery/provider-signal-webhook` is not owned by SubmissionManager.
   - `GET /v1/intents/{intentId}/delivery` is not owned by SubmissionManager.
   - `GET /v1/intents/{intentId}/delivery/history` is not owned by SubmissionManager.
7. Swagger compatibility must not weaken method constraints; unsupported methods continue to return method/route errors per existing behavior.

## Race conditions and handling (explicit)

The primary race is contract drift: runtime route behavior and documented operations can diverge when code and docs evolve independently across concurrent changes. This race must be handled with deterministic compatibility checks that fail verification when any owned route/method, status code, media type, parameter, or schema drifts from the canonical OpenAPI contract.

A secondary race exists around optional UI template enablement for `/ui/history`. The contract must declare the operation semantics when enabled, and verification must explicitly validate behavior in enabled and disabled-template runtime configurations so documentation remains accurate for both supported states.

A route-ownership race can occur when delivery-tracking routes are accidentally registered in SubmissionManager while docs are updated or reused. Verification must fail if SubmissionManager starts owning delivery-tracking endpoints or if such endpoints appear in the SubmissionManager OpenAPI contract.

## Failure semantics (explicit)

Swagger compatibility failures are release-blocking contract failures. If the OpenAPI document is missing, invalid, stale, or mismatched to owned runtime behavior, verification must fail and the change must not be considered complete.

Runtime request failure semantics for SubmissionManager endpoints remain as currently specified and implemented, including `invalid_request`, `not_found`, `idempotency_conflict`, `internal_error`, and method/route errors. Swagger compatibility must document these failures accurately and must not alter their meaning.

If `waitSeconds` is invalid, malformed, or negative, `POST /v1/intents` remains a `400 invalid_request` response with unchanged semantics. If `waitSeconds` is above the server maximum, clamping behavior remains unchanged and must be documented.

## Concurrency guarantees (explicit)

Swagger compatibility does not change SubmissionManager execution concurrency guarantees. Leader/follower execution rules and fencing invariants remain governed by `specs/submission-tracking/submission-manager-leaderlease.md`.

Concurrent HTTP callers must observe consistent schema-level behavior for the same endpoint semantics irrespective of instance role. Swagger documentation and verification must treat this consistency as a contract requirement.

Concurrent edits to code and API documentation must converge on one canonical OpenAPI contract per revision; verification must detect and reject mixed or partially updated contracts.

## Observable acceptance criteria (explicit)

Criterion 1: SubmissionManager has one canonical OpenAPI 3.0.x contract artifact at `specs/submission-tracking/submission-manager-openapi.yaml` that validates successfully with Swagger/OpenAPI validators and renders in Swagger UI without parser errors.

Criterion 2: The canonical contract contains every SubmissionManager-owned in-scope route/method and no SubmissionManager-non-owned delivery route/method.

Criterion 3: `POST /v1/intents` contract coverage includes `waitSeconds` query behavior, request body requirements (`intentId`, `submissionTarget`), `200` intent response, and error responses for malformed request, unknown target, idempotency conflict, and internal failures.

Criterion 4: `GET /v1/intents/{intentId}` and `GET /v1/intents/{intentId}/history` contract coverage includes success and not-found/error behaviors with schemas aligned to current runtime payloads.

Criterion 5: `GET /healthz`, `GET /readyz`, and `GET /metrics` are documented with correct response media types and method constraints; `/metrics` behavior when metrics are unavailable is documented.

Criterion 6: `/ui/history` form-post contract is documented with request media type (`application/x-www-form-urlencoded`) and HTML response behavior, including failure responses for invalid form data and unknown intent.

Criterion 7: Verification includes a deterministic drift check proving runtime route+method+status/media behavior matches the canonical OpenAPI contract for SubmissionManager.
