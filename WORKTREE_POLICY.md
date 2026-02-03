# Git Worktree Policy

## Objective

The purpose of using Git worktrees in this repository is **not productivity**, experimentation, or parallel feature development.

The purpose is to **enforce separation of authority** between:

* human decision-making, and
* Codex execution.

Worktrees exist to prevent:

* plan drift during execution,
* accidental scope introduction,
* contamination between mental modes,
* Codex acting outside its mandate.

---

## Core Principle

> **Worktrees isolate authority, not attention.**

A new worktree is created **only** when a different actor is allowed to make decisions, or when a different version of truth must remain frozen while another evolves.

---

## Defined Worktree Roles

### 1. PLAN Worktree

**Purpose**

* Planning, decision-making, intent definition.

**Primary actor**

* Human.

**Codex role**

* Assistant (clarification, questioning, critique only).

**Allowed activities**

* Creating or modifying specification documents.
* Creating or modifying plan documents.
* Defining scope, non-goals, constraints, and acceptance criteria.
* Surfacing risks and trade-offs.

**Disallowed activities**

* Writing or modifying production code.
* Writing or modifying tests.
* Refactoring code.
* Making execution-time decisions.

**Exit condition**

* All required decisions are explicit.
* Scope is frozen.
* A single unambiguous plan exists.

**Required marker**

* The execplan (or plan doc) must include an explicit **“PLAN-READY”** line before EXEC may begin.

---

### 2. EXEC Worktree

**Purpose**

* Deterministic execution of a frozen plan.

**Primary actor**

* Codex.

**Human role**

* Supervisor / reviewer.

**Authoritative inputs**

* Frozen specification documents.
* Frozen plan documents.

These documents are treated as **immutable contracts** during EXEC.

**Allowed activities**

* Implementing code exactly as specified.
* Writing tests required by the plan.
* Performing refactors explicitly listed in the plan.
* Making mechanical changes required to realize intent **and traceable to a plan line**.
* **Doc sync only:** update specs/README to reflect behavior already in the plan or already implemented.
  **Doc sync must not reinterpret, generalize, or add explanatory intent beyond what already exists.**

**Disallowed activities**

* Modifying specification or plan documents to introduce new scope.
* Making architectural or design decisions.
* Opportunistic refactors or “while we’re here” improvements.

---

## Sub-Modes Within EXEC (No New Worktrees)

The EXEC worktree supports multiple **sub-modes**, which do **not** justify separate worktrees.

### EXEC-IMPLEMENT

* Codex builds the feature as specified.

### EXEC-HARDEN

* Codex tightens documentation.
* Codex validates and improves test coverage.
* Codex refactors for clarity and idiomatic Go **only if behavior is unchanged**.
* **Refactors must be mechanically verifiable as behavior-neutral** (e.g., renaming, extraction, reformatting, control-flow-preserving changes).
  **If behavioral equivalence cannot be argued mechanically, execution must stop.**
* No new behavior or scope may be introduced.

### EXEC-REVIEW

* Codex reviews the result for correctness, completeness, and conformance.
* Review is read-only in intent.
* Review runs in a separate Codex session when non-trivial.

All EXEC sub-modes operate on the **same filesystem truth**.

---

## Stop Conditions (Mandatory)

During EXEC (any sub-mode), Codex must **stop immediately** and report if:

* A required decision is missing.
* The plan is ambiguous or contradictory.
* **More than one plausible interpretation exists.**
* New scope appears necessary.
* A design or architectural decision would be required.
* A spec conflict cannot be resolved by simple doc sync.
* A change cannot be mapped to a specific plan line.

In such cases:

* Execution halts.
* Control returns to PLAN.
* A new planning cycle begins.

---

## What Does *Not* Get a Worktree

The following do **not** justify separate worktrees:

* Coverage analysis.
* Refactoring for style or idiomatic standards (behavior-neutral only).
* Performance review.
* Documentation tightening.
* Code review.

These are **lenses applied within EXEC**, not changes in authority or truth.

---

## Session Discipline

* One Codex session must operate in **one role only**.
* Separate sessions are **recommended** for IMPLEMENT vs HARDEN vs REVIEW on non-trivial work.
* PLAN and EXEC must never share a Codex session.

Sessions isolate cognition; worktrees isolate authority.

---

## Summary Rule

> **Create a new worktree only when decision-making authority changes or when two truths must coexist.**

If neither condition holds, remain in the existing worktree.

---

## Non-Goals

This policy does not:

* prescribe Git commands,
* describe branching strategies,
* define repository layout,
* replace AGENTS.md or planning documents.

It defines **when worktrees exist and why**—nothing more.
