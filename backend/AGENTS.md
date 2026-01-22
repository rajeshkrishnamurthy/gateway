# AGENTS.md — Go Code Generation (Codex)

## Role
Generate Go code for a long-lived backend. Optimize for predictability, convergence, low review cost, and boring, obvious, idiomatic Go. Do not optimize for elegance, abstraction, or future flexibility.

## Global Defaults
- Prefer simple, flat, explicit code.
- Fewer functions > cleaner decomposition.
- Fewer abstractions > extensibility.
- Deletion is success.
- If unsure, choose the simplest working code.

## Control Flow
- Prefer early returns and guard clauses.
- Keep the happy path linear and near the bottom.
- Avoid deep nesting and single-exit patterns.
- Control flow must match execution order.

## Errors
- Errors are values.
- Handle errors locally and explicitly.
- Do not swallow errors.
- Do not log-and-continue unless explicitly required.
- Errors must rejoin main control flow.
- No global or centralized error handling.

## Abstraction & Interfaces
- Do not introduce interfaces unless multiple concrete implementations exist today.
- Do not design for hypothetical extensibility.
- Interfaces are consumer-defined, small, and local.
- Prefer concrete types.
- Inline simple logic; do not abstract guards or conditionals.

## Functions & Structure
- Helpers must justify themselves by clarity, not reuse.
- Avoid semantic grouping that hides behavior.
- Prefer locality over reuse.
- Code should be easy to inline or delete.

## Concurrency (Strict)
- Concurrency is suspicious by default.
- Do not add concurrency unless clearly justified.
- All goroutines must have explicit, bounded lifetimes.
- No fire-and-forget goroutines.
- All concurrent work must rejoin control flow.
- Errors from concurrent work must be surfaced.
- Concurrency boundaries must be obvious (`go` keyword).

## Tooling
- Code must be `gofmt` compliant.
- Use standard library only unless explicitly requested.
- Avoid adding dependencies.
- Use standard `testing` package only.
- Stable diffs > refactors.

## Comments (Strict)
- Do not comment obvious control flow or arithmetic.
- Prefer clearer naming or structure over comments.
- Comments explain why a constraint exists, not what the code does.
- Comments only for non-obvious invariants or limits.
- If a comment can be removed without loss of understanding, it should not exist.
- Comments may protect non-obvious business rules, product constraints, or external limits.
- Exported APIs must have intent-level comments.
- In Go, comments justify decisions; they do not narrate execution.
- Do not comment error variables, constants, or fields if names are self-explanatory.
- Do not add doc-comments by default; comments must earn their existence.
- When in doubt, remove the comment.

## SMS Provider Integration — Ubiquitous Language (Strict)
- provider: external SMS system (outside our control).
- ProviderCall: runtime callable capability.
  Type: `func(context.Context, SMSRequest) (ProviderResult, error)`.
  Invoked as: `providerCall(ctx, req)`.
- modelProviderCall: concrete builder that returns a ProviderCall.
- adapter: conceptual role only; adapter logic lives inside the ProviderCall implementation.
  Do not introduce adapter structs or interfaces unless duplication exists.
- gateway: domain service; owns validation and idempotency; never talks to providers directly, only invokes a ProviderCall.

Rules:
- Do not name ProviderCall variables as `provider`; use `providerCall`.
- Treat ProviderCall as executable authority, not an object.

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
- All HTTP route registration and handlers must live in `main.go` under `cmd/gateway/`.
- Core packages must not depend on `net/http`.
- UI handlers must be registered alongside other HTTP routes, not in core packages.

## Tests
- Tests should be boring and obvious.
- Test behavior, not implementation.
- Avoid mocks unless unavoidable.
- Tests should make deletion safer.

## Regeneration Rules
- Regeneration must not increase abstraction.
- Regeneration must not add layers or patterns.
- Regeneration must preserve or reduce complexity.
- Do not refactor for aesthetics.
- Prefer simplification over improvement.

## Reviewer Alignment
Assume the reviewer prioritizes explicit control flow, minimal abstraction, skimmability, and ease of deletion. Optimize for reviewer trust, not cleverness.
