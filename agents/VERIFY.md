# agents/VERIFY.md — VERIFY Mode

## Purpose

VERIFY exists to **challenge correctness and completeness** of the implemented feature against the frozen spec and the execplan.

VERIFY answers: **can we falsify the guarantees?**
It is not a mode for new requirements, design changes, or implementation exploration.

## Authority

* Primary actor: Codex (verifier)
* Human role: Minimal (only for escalations)
* VERIFY must run in a separate session from EXEC on non-trivial work.

## Inputs (Immutable Contracts)

* Frozen spec: `specs/<feature>.md` marked `EXEC-READY`
* execplan: `plans/<feature>-execplan.md` (living document as produced by EXEC under `backend/PLANS.md`)
* Implementation + tests produced by EXEC
* Repo-level Go discipline from `AGENTS.md` (boring, obvious, idiomatic Go; reviewer alignment) 

## Outputs

* Verification results (what was proven, what failed)
* Additional **meaningful** verification tests when required
* Coverage analysis as evidence (signal), not a goal
* Explicit escalations for missing/ambiguous intent

## Role of Existing Tests

Implementation-written tests are valuable inputs:

* They enable fast feedback and local correctness checks.
* They do not constitute proof of completeness.

VERIFY treats existing tests as a baseline and looks for gaps relative to the spec.

## Verification Activities (Allowed)

* Run the full test suite and any execplan-specified validation commands.
* Map each spec invariant/failure mode/race-handling rule to one or more tests or observations.
* Add verification tests that are:

  * adversarial (try to break guarantees),
  * spec-mapped (each test cites which invariant/failure mode it covers),
  * and aligned with boring, explicit Go tests. 
* Use coverage to identify untested areas and investigate whether they correspond to spec-critical behavior.

## Coverage Policy (Mandatory)

Coverage is a **diagnostic signal**, not a goal.

Rules:

* Do not add tests solely to raise coverage percentage.
* Every new verification test must map to at least one of:

  * a spec invariant,
  * a documented failure mode,
  * a concurrency/race condition,
  * a boundary condition implied by the spec.
* If uncovered code cannot be tested without defining new behavior, VERIFY must stop and escalate (SPEC/DESIGN).

## Test Style Constraints (From Repo Discipline)

* Tests should be boring and obvious.
* Test behavior, not implementation.
* Avoid mocks unless unavoidable. 
* Prefer explicit table tests only when they clarify behavior; do not generate parameter spam.

## Disallowed Activities (Hard Stop)

* Weakening or redefining spec guarantees
* Changing implementation behavior to “make tests pass”
* Introducing new requirements, scope, or semantics
* Refactoring production code (unless it is required solely to surface observability for verification and does not change behavior; otherwise escalate)
* Adding tests that exist only to increase coverage metrics

## Escalation Rules (Mandatory)

VERIFY must stop and escalate when:

* the spec is ambiguous or admits multiple interpretations,
* a required behavior is missing from the spec,
* correctness requires a new decision,
* a failure mode/race scenario cannot be tested without clarifying intent,
* verification reveals an architectural uncertainty (route to DESIGN).

If the human explicitly asks for a decision during VERIFY, Codex may decide but must still escalate to SPEC/DESIGN to record the decision.

Escalation targets:

* SPEC: semantics/invariants/acceptance criteria are unclear or missing
* DESIGN: mapping/convention choice is required for determinism

## Completion Criteria

VERIFY is complete when:

* spec invariants and acceptance criteria are demonstrably satisfied, and
* any uncovered, spec-relevant gaps are either:

  * covered by meaningful tests, or
  * escalated explicitly as missing intent.

VERIFY must record a brief summary of what was validated and any remaining risks/gaps.
