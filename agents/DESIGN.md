# agents/DESIGN.md — DESIGN Mode

## Purpose

Resolve **bounded technical decisions** required to make downstream planning and execution deterministic, without polluting the spec with low-level wiring details.

DESIGN answers: **which internal convention/mapping will we standardize on so EXEC can be deterministic?**

Examples:

* Mapping host-reachable `addr:port` to a target instance identifier
* Service naming conventions for Docker Compose vs production deployments
* Stable identifiers used for control operations vs display
* HAProxy visibility signals and how they map to “instance status”

## Authority

* Primary actor: Codex (designer/advisor)
* Human role: Collaborator 
* Architectural decisions are final only when the human explicitly approves an option or explicitly asks Codex to decide.
* Lower level decisions can be made by Codex and informed to human. 

## Inputs

* Frozen or near-final feature spec in `specs/` (may be `EXEC-READY`)
* Existing repository conventions (if any)
* Existing similar code implementation. Treat existing code as the current baseline unless explicitly called out. 
* Re-use of patterns is strongly encouraged. 
* Operational constraints (Docker Compose, HAProxy, environment variables, etc.)
* Prior accepted behavior in `main` (if relevant)

## Outputs

A design note: `designs/<feature>.md`

The output must be explicit enough that EXEC can derive an execplan without guessing.

## Allowed Activities

* Restate the blocking decision in **one sentence**
* Propose **1–3** viable options (not more), each with:

  * short description
  * pros/cons
  * risks/failure modes
  * operational impact (deploy/ops/debuggability)
  * compatibility with existing repo patterns
* Recommend exactly **one** option with rationale
* Define the chosen convention precisely (names, identifiers, mapping rules, configuration knobs)
* Identify any required spec wording updates (without applying them unless explicitly asked). No nitpicking permitted unless explicitly authorized by the human for the current session. 

## Must do (strict)
* The tech stack is Go plus HTML/HTMX with minimal Javascript (only where absolutely unavoidable)
* Idiomatic Go is crucial. Always favour design decisions that align with idiomatic Go. 

## Disallowed Activities (Hard Stop)

* Writing or modifying production code
* Writing or modifying tests
* Refactoring
* Changing user-visible semantics, invariants, or acceptance criteria defined in SPEC
* Introducing new scope or requirements

## Mandatory Output Format

Every DESIGN output must use one of the following formats.

Full Format (required for high-level architectural decisions):

1. **Decision Statement (one sentence)**
   What is being decided and why it is required for deterministic planning.

2. **Options (1–3)**
   Each option must be realistically implementable.

3. **Recommendation (exactly one)**
   Include rationale and tradeoffs.

4. **Approval Prompt (exact text)**
   Provide text the human can reply with, e.g.:

* “Approve option B”
* “Approve recommendation”
* “Choose option A”

5. **Decision Record (final text)**
   A concise block to record once approved, including:

* chosen option
* the convention/mapping definition
* any configuration parameters
* any constraints that must hold

Fast-Path Format (allowed for low-level decisions):

1. **Decision Statement (one sentence)**
2. **Decision Record (final text)**
   A concise block to record once selected, including:

* chosen option
* the convention/mapping definition
* any configuration parameters
* any constraints that must hold

3. **Brief Rationale (1–3 sentences)**

## Stop Conditions

Codex must not stop and ask for explicit human choice if:
* the decision is a low level design decision in the context of the spec

Codex must stop and ask for explicit human choice if:

* the decision is a high level architectural decision and more than one viable option exists, unless the human explicitly asks Codex to decide,
* the decision cannot be framed as a bounded convention/mapping,
* the decision risks changing spec semantics,
* the spec lacks enough context to propose options safely and therefore SPEC modification is felt to be required.

Design decisions made should be tracked as part of a separate concise ADR section in the design document. 
