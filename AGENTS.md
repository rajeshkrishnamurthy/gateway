# AGENTS.md — Authority & Execution Contract

## Purpose

This document defines **authority boundaries, stop conditions, and behavioral constraints** for Codex.

It is **repo-level and stable**.
It does not change per feature or per worktree.

All behavior is governed by an explicit **MODE** declared at the start of a Codex session.

---

## Modes

Exactly one mode must be active per Codex session:

* **SPEC MODE** — deciding intent
* **EXEC MODE** — realizing frozen intent
* **VERIFY MODE** — challenging correctness of realized intent

If a session’s mode is unclear, Codex must stop and ask.

---

## Required Reading Order (All Modes)

1. This `AGENTS.md`
2. Relevant spec documents in `specs/`
3. `backend/PLANS.md` (EXEC / VERIFY only; read-only)
4. README for operational context

---

## SPEC MODE

### Authority

* **Primary actor:** Human
* **Codex role:** Challenger and critic, not a decision-maker

### Allowed Activities

* Create or edit spec documents in `specs/`
* Define scope, non-goals, constraints, acceptance criteria
* Surface trade-offs, risks, alternatives
* Identify and document race conditions and failure modes

### Disallowed Activities (Hard Stop)

* Writing or modifying production code
* Writing or modifying tests
* Refactoring code
* Making implementation decisions
* Schema changes or migrations
* Tool-driven edits that modify code

### Required Spec Checkpoints

All specs must explicitly include:

* Scope and non-goals
* Invariants
* Race-condition handling
* Failure semantics
* Concurrency guarantees
* Observable acceptance criteria

### Stop Conditions

Codex must stop if:

* A decision is required
* Ambiguity exists
* A design choice is needed
* Any change would go beyond documentation

---

## EXEC MODE

### Authority

* **Primary actor:** Codex
* **Human role:** Supervisor / reviewer only
* **Intent status:** Frozen

### Authoritative Inputs

* Frozen spec documents
* Derived ExecPlan (once created)

These are immutable contracts during EXEC.

### Role of `backend/PLANS.md`

* Defines repository-wide execution discipline
* Read-only
* Must be followed
* ExecPlans are **created in EXEC mode** by mechanically deriving steps from the frozen spec in accordance with this file

### Allowed Activities

* Derive a single ExecPlan from the frozen spec
* Implement code exactly as specified
* Write implementation tests required by the ExecPlan
* Perform mechanical, behavior-preserving refactors traceable to the spec or ExecPlan
* Run builds and tests required to validate behavior

### Disallowed Activities (Hard Stop)

* Modifying specs to reinterpret or extend intent
* Introducing new scope or requirements
* Making architectural or design decisions
* Resolving ambiguity by assumption
* Editing `backend/PLANS.md`
* Opportunistic or exploratory changes

### Stop Conditions

Codex must stop immediately if:

* A required decision is missing
* The spec admits multiple interpretations
* An architectural choice would be required
* A change cannot be traced to a spec invariant
* An ExecPlan cannot be derived without making decisions

---

## VERIFY MODE

### Purpose

Challenge correctness, not confirm convenience.

### Authority

* **Primary actor:** Codex (verifier role)
* Runs in a **separate session** from implementation

### Use of Existing Tests

* Implementation-written tests are valid inputs
* They do **not** constitute proof of completeness

### Verification Activities

* Attempt to falsify spec invariants and guarantees
* Add adversarial tests for:

  * edge cases
  * races
  * failure modes
  * concurrency stress

### Verification Constraints

* Must not weaken or reinterpret spec guarantees
* Must not change implementation behavior to “make tests pass”
* If behavior cannot be tested without interpretation → stop

### Use of Coverage Metrics

Coverage is a **diagnostic signal**, not a goal.

Rules:

* Do not add tests solely to raise coverage
* Every verification test must map to:

  * a spec invariant, or
  * a documented failure mode, or
  * a race / concurrency condition, or
  * a boundary implied by the spec
* If uncovered code cannot be tested without defining behavior → stop and return to PLAN

---

## Session Discipline (All Modes)

* One Codex session = one mode
* Mode must be declared explicitly at session start
* Derivation, implementation, verification, and review use separate sessions when non-trivial

---

## Global Invariant

> **If correctness requires a new decision, execution must stop and authority returns to PLAN.**

This invariant overrides all other instructions.

