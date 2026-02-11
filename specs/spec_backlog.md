# Spec Backlog

## Purpose / Big Picture

This document records deferred specification work that is intentionally not part of the active DESIGN/EXEC slice in flight. The goal is to keep scope decisions explicit, reduce drift, and give future SPEC sessions a clear starting point.

## Scope

This backlog is for spec-level follow-up work only. It does not authorize implementation changes by itself, and it does not override active slice specs.

## Backlog Items

1. Post-EXEC delivery-tracking spec cleanup

After EXEC is complete for all delivery-tracking V1 slices, run one SPEC cleanup pass across `specs/delivery-tracking/` to remove stale wording, align cross-references, and ensure no contradictory or duplicate statements remain.

This pass should include umbrella and foundational docs, with special attention to any stale `Requires DESIGN decision` wording that may remain after completed design decisions have been absorbed.

2. Future fulfillment-tracking umbrella evolution

Current V1 is intentionally framed as delivery tracking. A future SPEC should introduce a broader fulfillment-tracking umbrella that can support multiple domain profiles, where delivery tracking is one profile and other profiles (for example payment or call-connection outcomes) can be added without duplicating core deterministic mechanics.

This work is deferred until after delivery-tracking V1 is fully implemented and stabilized, so generalization is informed by proven behavior rather than speculative abstraction.

## Usage Rule

When a backlog item is taken up, that work must be executed in a dedicated `MODE: SPEC` session and then moved into the appropriate normative spec documents.
