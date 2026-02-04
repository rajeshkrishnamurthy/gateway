# AGENTS.md — Mode Index and Global Invariants

## Purpose

This repository uses explicit **modes** to separate authority and reduce ambiguity when working with Codex.

`AGENTS.md` is an **index + constitution**:

* It defines global invariants and session rules.
* It delegates mode-specific rules to `agents/*.md`.

These rules are **normative**. Follow them to the letter.

---

## Modes

Exactly one mode must be active per Codex session:

* **SPEC** — define requirements and intent (what must be true)
* **DESIGN** — choose bounded technical conventions needed for determinism
* **EXEC** — plan and implement based on spec and design (if present). 
* **VERIFY** — adversarial validation and coverage-guided gap finding (no new decisions)
* **DEPLOY** - deployment related aspects
---

## Mode Declaration (Mandatory)

Every Codex session must begin with a single line:

`MODE: <SPEC|DESIGN|EXEC|VERIFY|DEPLOY>`

If the mode is unclear, Codex must stop and ask.

---

## Repository Document Locations

* Specs: `specs/`
* Design notes: `designs/`
* execplans: `plans/` (feature-specific execplan files)
* Execution discipline: `backend/PLANS.md` (read-only)

---

## Required Reading Order (All Modes)

1. `AGENTS.md`
2. Mode document: `agents/<MODE>.md`
3. Component-level `AGENTS.md` files for the area being changed (e.g., `backend/AGENTS.md` when working in `backend/`)
4. Relevant specs in `specs/`
5. Relevant design notes in `designs/` (when present)
6. Existing execplan in `plans/` (when present and relevant to the change)
7. README / operational docs (as needed)

---

## Global Invariants (Override All Modes)

1. **Human override on decisions.** If the human explicitly asks Codex to decide, Codex should decide, then follow mode constraints for where that decision is recorded/applied.
2. **No ambiguity-by-assumption.** If multiple interpretations exist for intent/behavior, stop and escalate. Low-level implementation ambiguity may be resolved and logged in the execplan.
3. **Specs are contracts.** Specs can be modified only in SPEC mode. 
4. **Plans are contracts.** execplans can be modified only in EXEC mode. 
5. **Verification cannot redefine intent.** VERIFY may not weaken or reinterpret spec guarantees.
6. **backend/PLANS.md is read-only.** It governs how execplans are written/executed; it must not be modified during feature work.

---

## Session Discipline (Mandatory)

* One Codex session = one mode.
* Non-trivial work must use separate sessions for:

  * EXEC (implement)
  * VERIFY (validate/review)

---

## Mode Transitions (Default Pipeline)

SPEC → DESIGN (optional) → EXEC → VERIFY

* Insist on DESIGN only when execplan cannot be deterministic without selecting a convention. 
* If EXEC or VERIFY discovers missing/ambiguous intent, return to SPEC/DESIGN and restart the cycle.
