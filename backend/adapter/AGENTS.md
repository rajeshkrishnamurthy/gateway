# Adapter AGENTS.md — ProviderCall Builders

## Purpose
This folder defines ProviderCall builders for external SMS and push providers. The SMS model adapter is the gold standard; push adapters must mirror its control flow and logging discipline.

## Ubiquitous Language (Strict)
- provider: external system (outside our control).
- ProviderCall: runtime callable capability. Signature matches the owning gateway request type.
- ProviderCall builder: function that constructs and returns a ProviderCall.
- adapter: the logic inside a ProviderCall; no adapter structs or interfaces.

## Required Behavior
- Translate gateway intent into provider request format.
- Perform the provider HTTP call using the passed context only.
- Interpret provider responses and map to ProviderResult or error.
- Log provider interactions and mapping decisions.

## Prohibited Behavior
- No retries, backoff, or time ownership beyond the passed context.
- No background goroutines.
- No state or persistence.
- No provider semantics surfaced to Gateway or callers.

## Logging (Strict)
Must log only:
- referenceId
- provider name
- provider endpoint or name
- provider HTTP status
- provider error codes (if any)
- mapping decision (provider outcome → gateway reason)
- redacted recipient/token
- messageLen (length only)
- messageHash (hash only)

Must not log:
- full recipient/token or full message text
- raw request payloads
- credentials, auth headers, or provider secrets

## Structure & Naming
- Keep code flat and explicit; no extra layers.
- Builder functions should be named `XProviderCall`.
- Provider name constants must be stable and used in logs.
- Use only standard library and `gateway` types.
- Do not name ProviderCall variables as `provider`; use `providerCall`.

## Tests
- Adapter-specific tests live alongside adapters in this folder.
- Test request translation, response parsing, and mapping to ProviderResult.

## Adding a Provider
- Copy `model_provider_call.go` verbatim and rename the file and builder function.
- Make the smallest possible edits to support the provider’s request format, response interpretation, and provider name.
- Preserve logging structure and fields; only substitute provider-specific values. Do not add or remove logged data.
- URL-encode dynamic query parameter values when building provider URLs.
- Add adapter-level tests alongside the implementation (request mapping, response handling, and outcome mapping).
- Read provider secrets (API keys) from provider-specific environment variables in the gateway `main.go`, not from config files.
- Map provider outcomes deterministically:
  - return `ProviderResult` only when the submission outcome is certain
  - return `error` for any ambiguous, transport, or provider failure
- Wire the provider in the owning gateway `main.go`; provider-specific constructor arguments are expected and must remain confined to bootstrap wiring.
