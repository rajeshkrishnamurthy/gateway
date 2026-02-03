# AGENTS.md — EXEC Worktree

## Scope
Applies to the EXEC worktree only. This worktree exists to execute frozen intent deterministically.

## Required reading order
1) Closest AGENTS.md
2) Frozen spec documents in `specs/`
3) `backend/PLANS.md` (execution discipline; read-only)
4) README for operational context

## Authority mode
EXEC mode is active.

## Primary actor
Codex.

## Human role
Supervisor and reviewer only. No new decisions are made in EXEC.

## Authoritative inputs
- Frozen specification documents (`PLAN-READY`)
- Derived ExecPlan (once created)

These inputs are immutable contracts during EXEC.

## Role of backend/PLANS.md
`backend/PLANS.md` defines repository-wide execution discipline.
It is read-only and must be followed.
ExecPlans are feature-specific artifacts that are created in EXEC mode by deriving steps from the frozen spec in accordance with `backend/PLANS.md`.

## Allowed activities
- Derive a single ExecPlan from the frozen spec.
- Implement code exactly as specified in the spec and ExecPlan.
- Write tests required by the ExecPlan.
- Perform mechanical refactors that preserve behavior and are traceable to the spec or ExecPlan.
- Run builds and tests required to validate behavior.

## Disallowed activities (hard stop)
- Modifying spec documents to reinterpret or extend intent.
- Introducing new scope, requirements, or constraints.
- Making architectural or design decisions.
- Resolving ambiguity by assumption.
- Editing `backend/PLANS.md`.
- Editing AGENTS.md during execution.
- Opportunistic or exploratory changes.

## Stop conditions (mandatory)
Codex must stop immediately and report if:
- a required decision is missing,
- the spec is ambiguous or admits multiple interpretations,
- an architectural or design choice would be required,
- a change cannot be traced to a spec statement or invariant,
- the ExecPlan cannot be derived without making new decisions.

## Session discipline
- One Codex session must operate in EXEC role only.
- ExecPlan derivation, implementation, hardening, and review must use separate sessions when non-trivial.

## Output expectations
- All changes must be deterministic and traceable.
- No intent drift.
- No silent assumptions.

