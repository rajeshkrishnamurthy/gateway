# AGENTS.md — Go Code Generation (Codex)

## Role
You generate Go code for a long-lived backend.
Your goal is predictability, convergence, and low review cost.
Optimize for boring, obvious, idiomatic Go.

Do NOT optimize for elegance, abstraction, or future flexibility.

---

## Global Defaults
- Prefer simple, flat, explicit code.
- Fewer functions > cleaner decomposition.
- Fewer abstractions > extensibility.
- Deletion is success.
- If unsure, choose the simplest working code.

---

## Control Flow
- Prefer early returns and guard clauses.
- Keep the happy path linear and near the bottom.
- Avoid deep nesting and single-exit patterns.
- Control flow must match execution order.

---

## Errors
- Errors are values.
- Handle errors locally and explicitly.
- Do not swallow errors.
- Do not log-and-continue unless explicitly required.
- Errors must rejoin main control flow.
- No global or centralized error handling.

---

## Abstraction & Interfaces
- Do NOT introduce interfaces unless multiple concrete implementations exist today.
- Do NOT design for hypothetical extensibility.
- Interfaces are consumer-defined, small, and local.
- Prefer concrete types.
- Inline simple logic; do not abstract guards or conditionals.

---

## Functions & Structure
- Helpers must justify themselves by clarity, not reuse.
- Avoid semantic grouping that hides behavior.
- Prefer locality over reuse.
- Code should be easy to inline or delete.

---

## Concurrency (Strict)
- Concurrency is suspicious by default.
- Do not add concurrency unless clearly justified.
- All goroutines must have explicit, bounded lifetimes.
- No fire-and-forget goroutines.
- All concurrent work must rejoin control flow.
- Errors from concurrent work must be surfaced.
- Concurrency boundaries must be obvious (`go` keyword).

---

## Tooling
- Code must be `gofmt` compliant.
- Use standard library only unless explicitly requested.
- Avoid adding dependencies.
- Use standard `testing` package only.
- Stable diffs > refactors.

---
## Comments (Strict)

Do not comment obvious control flow or arithmetic.
Prefer clearer naming or structure over comments.
Comments may explain why a constraint exists, not what the code does.
Use comments to document non-obvious invariants or limits only.
If a comment can be removed without loss of understanding, it should not exist.
Comments are permitted to protect non-obvious business rules, product constraints, or externally imposed limits that must survive refactoring or regeneration.
Public (exported) APIs must have intent-level comments.
In Go, comments justify decisions — they do not narrate execution.
Do NOT comment error variables, constants, or fields if their names are self-explanatory.
Do NOT add doc-comments by default; comments must earn their existence.
When in doubt, remove the comment!

---

## HTML UI Handlers (Strict)

- HTML UI handlers are pure adapters.
- They MUST call existing core logic and must not reimplement behavior.
- Handlers MUST only perform input parsing, core invocation, and template selection.
- Any data shaping beyond trivial field assignment belongs in the core, not the handler.
- UI handlers MUST render existing HTML templates using html/template.
- HTML templates are located under ../http/ui/ and MUST NOT be duplicated or moved.
- Templates MUST receive explicit view structs; no implicit or global data.
- Handlers MUST NOT embed business rules or decision logic.
- Templates MUST be treated as fragments, not documents.
- JSON APIs and HTML UI handlers MUST share the same core logic, never duplicate it.
- Internal errors MUST be mapped to user-facing messages before rendering.

---

## HTTP Handler Placement (Strict)

- All HTTP route registration and handlers MUST live in main.go under cmd/gateway/.
- Core packages MUST NOT depend on net/http.
- UI handlers MUST be registered alongside other HTTP routes, not in core packages.

---
## Tests
- Tests should be boring and obvious.
- Test behavior, not implementation.
- Avoid mocks unless unavoidable.
- Tests should make deletion safer.

---

## Regeneration Rules
- Regeneration must not increase abstraction.
- Regeneration must not add layers or patterns.
- Regeneration must preserve or reduce complexity.
- Do not refactor for aesthetics.
- Prefer simplification over improvement.

---

## Reviewer Alignment
Assume the reviewer prioritizes:
- explicit control flow
- minimal abstraction
- skimmability
- ease of deletion

Optimize for reviewer trust, not cleverness.

