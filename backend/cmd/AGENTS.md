# AGENTS.md - cmd

## Scope
Rules for HTTP entrypoints under `cmd/`.

## HTML UI Handlers (Strict)
- HTML UI handlers are pure adapters.
- Must call existing core logic and must not reimplement behavior.
- Must only perform input parsing, core invocation, and template selection.
- Any data shaping beyond trivial field assignment belongs in the core.
- Must render existing HTML templates using `html/template`.
- Templates are located under `../ui/` and must not be duplicated or moved.
- Templates must receive explicit view structs; no implicit or global data.
- Handlers must not embed business rules or decision logic.
- Templates must be treated as fragments, not documents.
- JSON APIs and HTML UI handlers must share the same core logic, never duplicate it.
- Internal errors must be mapped to user-facing messages before rendering.

## HTTP Handler Placement (Strict)
- HTTP route registration and handlers must live in the `cmd/<service>` package and may be split across files (for example `handlers.go`, `routes.go`).
- `main.go` must be limited to wiring dependencies and starting the server.
- Core packages must not depend on `net/http`.
- UI handlers must be registered alongside other HTTP routes, not in core packages.
