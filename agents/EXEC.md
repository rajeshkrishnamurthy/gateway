# agents/EXEC.md — EXEC Mode (execplan + Implement)

## Purpose

EXEC is a single mode in which Codex:

1. authors a feature-specific execplan (as an in-process, living document), and then
2. implements the feature by following that execplan.

EXEC answers: **turn frozen intent into working behavior, with evidence**.

EXEC must follow `backend/PLANS.md` to the letter for:

* what an execplan must contain,
* how it must be structured,
* and how it is maintained while implementation proceeds.
Where `backend/PLANS.md` calls for resolving ambiguity in the plan, this applies only to low-level implementation ambiguity; intent/behavior ambiguity must be escalated.

## Authority

* Primary actor: Codex (executor)
* Human role: Minimal / supervisor only (intervene only on stops/escalations)

No new user-visible requirements may be introduced in EXEC.

## Inputs (Immutable / Read-only)

* Frozen spec: `specs/<feature>.md` marked `EXEC-READY`
* Design note (when present): `designs/<feature>.md` (approved conventions/mappings)
* `backend/PLANS.md` (execution discipline; read-only)

## Outputs

* execplan: `plans/<feature>-execplan.md`
* Production code + tests implementing the spec
* Operational wiring (docker-compose, HAProxy config, etc.) only if required by the spec or execplan
* Evidence: test output / logs / transcripts as required by the execplan

## File Locations

* Specs: `specs/`
* Design notes: `designs/`
* execplans: `plans/`
* Execution discipline: `backend/PLANS.md` (read-only)

---

## EXEC Phases (within a single EXEC session, unless you choose to split)

### Phase 1 — execplan Authoring

Goal: Produce a deterministic execplan that a novice could follow from the file alone.

Rules:

* Create or update `plans/<feature>-execplan.md`.
* The execplan must be fully self-contained and novice-guiding.
* The execplan must include and maintain the required living sections (Progress, Decision Log, Surprises & Discoveries, Outcomes & Retrospective, etc.) as defined by `backend/PLANS.md`.
* The execplan must reference the frozen spec and any relevant design note decisions.
* The execplan must not introduce new intent. It may only expand and sequence work implied by the frozen inputs.

Marker:

* When the execplan is complete enough to begin implementation, add `EXECPLAN-READY` near the top of the execplan.

### Phase 2 — Implementation (Execute the execplan)

Goal: Implement milestone-by-milestone, keeping the execplan current.

Rules:

* Follow the execplan steps in order unless the execplan itself instructs otherwise.
* Update `Progress` at every stopping point.
* Record any low-level decisions in the execplan `Decision Log`.
* Capture unexpected behaviors in `Surprises & Discoveries` with concise evidence.
* Commit frequently in small, reviewable chunks (as the discipline expects).

---

## Allowed Activities

* Author and maintain the execplan (per `backend/PLANS.md`).
* Implement code exactly as required by the frozen spec and derived execplan.
* Write implementation tests that are required by the execplan and that validate stated spec acceptance criteria.
* Run builds and tests required by the execplan.
* Perform mechanical refactors only when behavior is preserved and the change is necessary to complete the plan.

## Disallowed Activities (Hard Stop)

* Modifying the frozen spec (anything marked `EXEC-READY` is immutable in EXEC).
* Introducing new scope, new requirements, or new user-visible behavior not present in spec/design.
* Making architectural or design decisions that change semantics.
* Resolving ambiguity by assumption for intent/behavior. Low-level implementation ambiguity may be resolved and logged in the execplan.
* Editing `backend/PLANS.md`.
* Large opportunistic refactors not required by the plan.
* Prototyping/spikes unless explicitly called out in the execplan and justified as de-risking.

---

## Decision Policy (What EXEC may decide vs must escalate)

EXEC may decide ONLY low-level, implementation-local details that do not alter semantics and can be justified as mechanical choices, such as:

* step ordering within a milestone,
* file/func placement consistent with existing patterns,
* naming consistent with existing conventions,
* small refactors needed to make the plan executable.

If the human explicitly asks EXEC to decide, Codex may decide, but must still honor stop conditions and route any intent-changing decisions to SPEC/DESIGN for recording.

EXEC must escalate and stop (see next section) when:

* a user-visible behavior is unclear,
* an invariant/failure mode/concurrency guarantee is underspecified,
* multiple viable internal conventions/mappings exist and are not decided in `designs/<feature>.md`,
* any decision would change or reinterpret intent.

---

## Stop Conditions (Mandatory)

EXEC must stop immediately and report when any of the following occurs:

1. Spec ambiguity:

* more than one plausible interpretation of behavior/invariants exists,
* an acceptance criterion cannot be implemented without deciding new behavior.

Action: return to SPEC mode (or human decision), update spec, then restart EXEC.

2. Convention/mapping ambiguity:

* a required mapping/convention is needed (e.g., identifier mapping) and multiple viable strategies exist,
* and no approved DESIGN decision exists.

Action: return to DESIGN mode, record the decision in `designs/<feature>.md` (or the spec’s conventions section), then restart EXEC.

3. Plan/step ambiguity:

* an execplan step cannot be executed as written,
* or it requires a decision that changes semantics.

Action: stop; if it is purely mechanical, amend execplan Decision Log and proceed; otherwise escalate to SPEC/DESIGN.

---

## Testing Notes (within EXEC)

* Implementation tests are encouraged and useful for rapid feedback.
* They do not constitute proof of completeness; adversarial verification belongs to VERIFY mode.

EXEC must not add “mindless” tests solely to improve coverage metrics.

---

## Completion

EXEC is complete when:

* the execplan milestones are fully implemented,
* validation steps pass,
* and the execplan is updated to reflect final outcomes and evidence.

Any remaining gaps or semantic questions must be escalated (SPEC/DESIGN), not patched silently.
