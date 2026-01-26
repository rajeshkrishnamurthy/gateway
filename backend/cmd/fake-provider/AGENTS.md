# AGENTS.md — fake-provider

## Scope
Binaries under `cmd/fake-provider/*` are local-only fake provider servers used for smoke tests and manual demos.

## Boundaries
- Not production code; do not reuse in gateways.
- Keep behavior minimal and stable.
- Keep endpoints aligned with the corresponding gateway adapters.
- Avoid new dependencies; use the standard library only.
- Do not log secrets or full PII.

## Changes
- If an adapter request/response shape changes, update the matching fake provider to stay aligned.
- Prefer explicit, single-file handlers over abstractions.
