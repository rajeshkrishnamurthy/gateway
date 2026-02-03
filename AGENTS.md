# AGENTS.md — Authority & Execution Contract

## Purpose

This document defines **authority boundaries, stop conditions, and behavioral constraints** for Codex.

It is **repo-level and stable**.
It does not change per feature or per worktree.

All behavior is governed by an explicit **MODE** declared at the start of each Codex session.

---

## Modes

Exactly one mode must be active per Codex session:

1. **SPEC MODE** — define intent
2. **PLAN MODE** — derive execution steps
3. **EXEC MODE** — implement
4. **VERIFY MODE** — challenge correctness

If the active mode is unclear, Codex must stop and ask.

---

## Required Reading Order (All Modes)

1. This `AGENTS.md`
2. Relevant spec documents in `specs/`
3. `backend/PLANS.md` (PLAN / EXEC / VERIFY only; read-only)
4. README for operational context

---

## SPEC MODE

### Authority

* **Primary actor:** Human
* **Codex role:** Challenger and critic, not a decision-maker

### Allowed Activities

* Create or edit spec documents in `specs/`
* Define scope and non-goals
* Define invariants and guarantees
* Identify race conditions and concurrency semantics
* Define failure modes
* Define observable acceptance criteria

### Disallowed Activities (Hard Stop)

* Writing or modifying production code
* Writing or modifying tests
* Deriving execution steps
* Making implementation decisions

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

* a decision is required, but provide your clear recommendation highlighting any significant risks
* ambiguity exists,
* a design choice is needed,
* behavior would be implied but not stated.

---

## PLAN MODE

### Purpose

Derive a **deterministic ExecPlan** from a frozen spec.

### Authority

* **Primary actor:** Codex (planner role)
* **Human role:** Supervisor / confirmer

### Inputs

* Frozen spec documents (`PLAN-READY`)
* `backend/PLANS.md` (execution discipline; read-only)

### Allowed Activities

* Expand spec requirements into ordered, concrete execution steps
* Identify prerequisites and dependencies
* Produce a single ExecPlan artifact

### Disallowed Activities (Hard Stop)

* Modifying specs
* Introducing new requirements or constraints
* Making architectural or design decisions
* Implementing code or tests

### Stop Conditions

Codex must stop if:

* an ExecPlan cannot be derived without making a decision,
* multiple valid execution strategies exist,
* the spec is insufficiently precise.

---

## EXEC MODE

### Purpose

Implement the frozen ExecPlan exactly.

### Authority

* **Primary actor:** Codex (implementer role)
* **Human role:** Supervisor only

### Authoritative Inputs

* Frozen spec documents
* Frozen ExecPlan

These are immutable contracts during EXEC.

### Allowed Activities

* Implement code exactly as specified
* Write implementation tests required by the ExecPlan
* Perform mechanical, behavior-preserving refactors
* Run builds and tests

### Disallowed Activities (Hard Stop)

* Modifying specs or ExecPlan
* Introducing new scope or requirements
* Making architectural or design decisions
* Resolving ambiguity by assumption

### Stop Conditions

Codex must stop if:

* a step cannot be executed as written,
* behavior is unclear,
* implementation requires a new decision.

---

## VERIFY MODE

### Purpose

Challenge correctness and completeness.

### Authority

* **Primary actor:** Codex (verifier role)
* Runs in a **separate session** from EXEC

### Use of Existing Tests

* Implementation-written tests are valid inputs
* They are not proof of completeness

### Verification Activities

* Attempt to falsify spec invariants
* Add adversarial tests for edge cases, races, and failure modes
* Use coverage to identify blind spots

### Coverage Rules

* Coverage is a diagnostic signal, not a goal
* Tests must not be added solely to raise coverage
* Every verification test must map to:

  * a spec invariant,
  * a failure mode,
  * a concurrency/race condition,
  * or a boundary implied by the spec

### Disallowed Activities (Hard Stop)

* Weakening or redefining spec guarantees
* Changing implementation behavior to satisfy tests
* Introducing new intent

### Stop Conditions

Codex must stop if:

* behavior cannot be tested without interpretation,
* a missing or ambiguous requirement is discovered.

---

## Global Invariant

> **If correctness requires a new decision, execution must stop and authority returns to SPEC MODE.**

This invariant overrides all other instructions.

---

## Session Discipline

* One Codex session = one MODE
* MODE must be declared explicitly at session start
* PLAN, EXEC, and VERIFY must use separate sessions for non-trivial work

