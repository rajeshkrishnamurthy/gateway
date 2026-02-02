# AGENTS.md - Go Code Generation (Codex)

## Role
Generate Go code for a long-lived backend. Optimize for predictability, convergence, low review cost, and boring, obvious, idiomatic Go. Do not optimize for elegance, abstraction, or future flexibility.

# ExecPlans
When writing complex features or significant refactors, use an ExecPlan (as described in `backend/PLANS.md`) from design to implementation.

## Global Defaults
- Prefer simple, flat, explicit code.
- Fewer functions > cleaner decomposition.
- Fewer abstractions > extensibility.
- Deletion is success.
- If unsure, choose the simplest working code.
- Keep application code runtime-agnostic; Docker/Compose specifics belong in ops configs, not in Go packages.

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

## Specs and README
- Treat `backend/spec/` as canonical for system semantics.
- When behavior or semantics change, update the relevant spec/README in the same change.

## Comments (Strict, drift-resistant)
- Purpose: comments must capture intent or constraints that are not obvious from code alone. Anything else belongs in specs/README.
- Package-level doc comments (for godoc) are allowed and should summarize module purpose and boundaries.
- Exported API comments (capitalized identifiers) are allowed and should describe behavior and invariants.
- Inline comments are allowed only for these topics:
  - Concurrency/locking intent: why a lock is held or released at a specific point.
  - Queue semantics: ordering rules, fairness, and why seq is used for FIFO.
  - Policy vs outcome: why a decision is made (for example, deadline check only for accepted).
  - Idempotency invariants: what constitutes conflict and why it is strict.
  - Non-obvious constraints: invariants like “contract snapshot must not change after submit.”
  - Flow intent (limited): a single-line header at the start of a complex function (roughly >40 LOC or multiple branches) describing the goal in one sentence. No step-by-step narration.
- Forbidden:
  - Explaining Go syntax.
  - Restating code (“set x to y”, “loop over items”).
  - Multi-line walkthroughs.
- Drift prevention:
  - Any behavior change must update affected comments in the same diff.
  - If a comment references a policy or constraint, it must remain aligned with the relevant spec; prefer brief “see <spec file>” references.

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
