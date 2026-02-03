# Model SMS Provider Adapter — Canonical Specification

## Purpose

This document specifies a canonical model SMS provider adapter for the Gateway.

The goal of this adapter is not to immediately support a real vendor, but to:

- validate the Gateway’s submission-only contract
- validate the sufficiency of ProviderCall and ProviderResult
- exercise real-world provider quirks and edge cases
- serve as the reference model for all future provider adapters

This adapter defines the shape, responsibilities, and boundaries of provider adapters.

---

## Adapter Responsibility

The adapter is responsible for all provider-specific behavior, including:

- translating Gateway intent into a provider-specific request format
- performing the provider HTTP call
- interpreting provider HTTP responses
- mapping provider outcomes into ProviderResult or error
- logging provider interaction and interpretation decisions

The adapter must not:

- retry requests
- own time beyond the passed context
- spawn background goroutines
- maintain state
- expose provider semantics to Gateway or callers

---

## Gateway ↔ Adapter Contract

### Adapter Signature

The adapter implements:

ProviderCall(ctx context.Context, req SMSRequest) (ProviderResult, error)

### Semantics

- Returning an error indicates the submission outcome is unknown
  (network failure, timeout, malformed response, panic).
- Returning a ProviderResult indicates the provider responded and was interpreted.

Gateway owns:

- timeouts
- panic containment
- normalization to SMSResponse

---

## Provider Request Mapping

### Gateway Input

SMSRequest contains:

- referenceId
- to
- message
- tenantId (optional)

### Model Provider Request

The model provider accepts:

- HTTP method: POST
- Endpoint: /sms/send
- Headers:
  - Content-Type: application/json
  - X-Request-Id: <referenceId> (optional, best-effort)
- Body (JSON):

{
  "destination": "<to>",
  "text": "<message>"
}

Notes:

- referenceId is passed as a header for correlation only.
- The provider is not required to persist or echo it back.
- tenantId is ignored by the provider.

---

## Provider Response Interpretation

The provider may return the following responses.

### Successful Acceptance

HTTP 200  
Body:

{
  "status": "OK",
  "provider_id": "abc123"
}

Adapter behavior:

- Interpret as accepted
- Return ProviderResult with Status = "accepted"
- provider_id may be logged but must not be surfaced

---

### Invalid Recipient (Definitive Rejection)

HTTP 400  
Body:

{
  "error": "INVALID_RECIPIENT"
}

Adapter behavior:

- Interpret as definitive rejection
- Return ProviderResult with:
  - Status = "rejected"
  - Reason = "invalid_recipient"

---

### Invalid Message (Definitive Rejection)

HTTP 400  
Body:

{
  "error": "INVALID_MESSAGE"
}

Adapter behavior:

- Interpret as definitive rejection
- Return ProviderResult with:
  - Status = "rejected"
  - Reason = "invalid_message"

---

### Provider-Side Failure

HTTP 500  
Any body or empty body

Adapter behavior:

- Submission outcome is unknown
- Return error

Gateway will normalize this as provider_failure.

---

### Timeout or Network Failure

Examples:

- context deadline exceeded
- connection refused
- DNS failure

Adapter behavior:

- Return error

Gateway will normalize this as provider_failure.

---

### Malformed or Unexpected Response

Examples:

- invalid JSON
- missing required fields
- unknown status values

Adapter behavior:

- Return error

Gateway will normalize this as provider_failure.

---

## Logging Expectations

The adapter must log:

- provider request attempt (sanitized)
- provider HTTP status
- provider response body or a safe summary
- parsing errors
- interpretation decisions such as:
  - mapped INVALID_RECIPIENT to invalid_recipient
  - mapped HTTP 500 to provider_failure

The adapter must not log:

- full message content
- sensitive headers or credentials

Gateway logging will capture:

- normalized outcome
- final submission result

---

## Panic Safety

The adapter must assume provider responses are untrusted.

The adapter:

- may panic due to bugs or malformed input

Gateway:

- will recover panics at the provider boundary
- will normalize panics to provider_failure

Adapters should avoid spawning goroutines.

If goroutines are used, the adapter must recover panics internally.

---

## Non-Goals

This adapter does not support:

- retries
- backoff
- delivery receipts
- reconciliation
- persistence
- asynchronous behavior
- multiple provider endpoints

These are intentionally out of scope.

---

## Canonical Mapping Rules

Provider accepted  
→ ProviderResult Status = "accepted"  
→ Gateway response status = accepted

Provider rejected with invalid recipient  
→ ProviderResult Status = "rejected", Reason = "invalid_recipient"  
→ Gateway response rejected / invalid_recipient

Provider rejected with invalid message  
→ ProviderResult Status = "rejected", Reason = "invalid_message"  
→ Gateway response rejected / invalid_message

Provider timeout, network error, HTTP 5xx, malformed response  
→ Adapter returns error  
→ Gateway response rejected / provider_failure

Unknown or unexpected provider behavior  
→ Adapter returns error  
→ Gateway response rejected / provider_failure

---

## Guiding Principle

Adapters translate protocol into meaning.

Gateway enforces meaning and boundaries.

This adapter is the reference against which all future adapters are judged.

---

## Next Step

Freeze this specification.

Then ask Codex to propose an implementation plan that:

- fits the current Gateway architecture
- does not require Gateway changes
- does not introduce new abstractions
- treats this adapter as canonical
