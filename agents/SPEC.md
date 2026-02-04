# agents/SPEC.md — SPEC Mode

## Purpose

Define **requirements and intent**. SPEC answers: **what must be true**.

SPEC produces a spec document under `specs/` that is:

* unambiguous,
* testable in principle,
* and sufficient for deterministic EXEC downstream.

## Authority

* Primary actor: Human
* Codex role: challenger/critic/advisor, not a decision-maker unless explicitly asked by the human

Codex may propose alternatives and highlight risks, but **final decisions belong to the human**.

## Inputs

* Existing specs in `specs/`
* Operational context from README/docs as needed
* Prior accepted behavior in `main` (if relevant)

## Outputs

* A feature spec in `specs/<feature>.md` (Markdown)

## Allowed Activities

* Create or edit spec documents in `specs/`
* Surface risks, trade-offs, and alternatives with clear recommendations
* Identify where a bounded convention/mapping is required and explicitly flag it for DESIGN
* Make spec-level decisions when explicitly asked by the human, and record them in the spec.

## Disallowed Activities (Hard Stop)

* Writing or modifying production code
* Writing or modifying tests
* Refactoring
* Deriving an execplan (that is EXEC mode)
* Making implementation decisions (names of functions, exact file edits, command sequences, etc.)

## Required Spec Checkpoints (Mandatory Sections)

Every spec must explicitly include:

1. Purpose / Big picture
2. Scope
3. Non-goals
4. Invariants (explicit)
5. Race conditions and handling (explicit)
6. Failure semantics (explicit)
7. Concurrency guarantees (explicit)
8. Observable acceptance criteria (explicit)

## DESIGN Hand-off Trigger

If the spec requires a bounded internal convention/mapping (e.g., naming, identifiers, address-to-instance mapping) and there are multiple viable strategies:

* SPEC must not guess.
* SPEC must explicitly record: “Requires DESIGN decision: <one sentence>.”
* Then stop and enter DESIGN mode.

## EXEC-READY Marker

When the spec is complete and frozen, add a single line near the top:

`EXEC-READY`

After `EXEC-READY`, the spec is a contract. It must not be modified outside SPEC mode. 

## Stop Conditions

Codex must stop and ask the human if:

* a required decision is missing,
* more than one plausible interpretation exists,
* a design choice is needed but not stated,
* acceptance criteria are not observable/verifiable,
* a bounded convention is required (enter DESIGN mode).

## Output Expectations

The spec must be:

* for non-complex requirements, the feature spec must be clear enough to be expanded into an execplan without guessing,
* for complex requirements, the feature spec taken with the design document must be clear enough to be expanded into an execplan without guessing. 
* explicit about races and failure behavior,
* written so that a verifier can map tests back to spec statements.
