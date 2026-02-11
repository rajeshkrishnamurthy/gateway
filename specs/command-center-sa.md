# Command Center: SubmissionManager, Delivery Tracking, and HAProxy services
COMPLETED
EXEC-READY

## Purpose

The Command Center must list SubmissionManager instances, delivery-tracking instances, and the HAProxy service in one operations view. Operators must be able to use the same start/stop interaction model for delivery-tracking instances that already exists for other instance-scoped services. The Admin Portal must expose the same Command Center service list and controls through its proxy view.

## Scope

In scope:

- Command Center configuration requirements for multiple SubmissionManager instances.
- Command Center configuration requirements for multiple delivery-tracking instances.
- Command Center configuration requirements for HAProxy process control.
- Start/stop actions for configured instances and services in the Command Center UI.
- Health visibility for each configured instance in the Command Center UI.
- Visibility of the same Command Center list and controls in the Admin Portal proxy view.

## Non-goals

Out of scope:

- Changes to SubmissionManager execution semantics or leader lease behavior.
- Changes to delivery-tracking domain semantics (`delivery status`, `delivery freshness`, or correlation behavior).
- Changes to HAProxy routing behavior or configuration syntax.
- New Admin Portal navigation, page layout, or shell behavior.
- New health endpoints; the Command Center must use configured `healthUrl` values.

## Invariants

- Command Center health state must be derived only from the configured `healthUrl` for each instance.
- Start/stop commands must remain the only control-plane actions exposed by the Command Center.
- Service and instance identity must be defined by Command Center config, not runtime discovery.
- The UI must display each configured `addr` value verbatim, without host/port derivation.
- Delivery-tracking instances must use the same ON/OFF start-stop behavior as other per-instance services.
- The Admin Portal Command Center proxy must not filter, rename, or suppress configured services; it must present the upstream Command Center list as-is (except embed-only chrome changes such as theme-toggle hiding).

## Configuration

This spec relies on the existing Command Center schema in `specs/services-health.md`. No schema changes are required.

Required service entries:

- One service entry per SubmissionManager instance, each with exactly one instance.
- One service entry per delivery-tracking instance, each with exactly one instance.
- One HAProxy service entry that may list multiple instances while retaining single-process toggle behavior.

Each instance must define:

- `name` (unique within the service)
- `addr` (used for display and command substitution)
- `healthUrl` (2xx means healthy)

Each SubmissionManager and delivery-tracking service entry must define `startCommand` and `stopCommand` so Command Center can execute per-instance stop/start actions.

SubmissionManager naming convention (required):

- Service `id` must be `submission-manager-<n>`.
- Service `label` must be `Submission Manager (<n>)`.
- Instance `name` must be `submission-manager-<n>`.

Delivery-tracking naming convention (required):

- Service `id` must be `delivery-tracking-<n>`.
- Service `label` must be `Delivery Tracking (<n>)`.
- Instance `name` must be `delivery-tracking-<n>`.

HAProxy toggle behavior (required):

- The HAProxy service must continue using `singleToggle` with a configured `toggleInstance`.

## UI behavior

- The Command Center UI must list all configured SubmissionManager instances, delivery-tracking instances, and HAProxy instances.
- Each listed instance must display current health based on `healthUrl`.
- The UI must show the configured `addr` string for every instance.
- SubmissionManager and delivery-tracking services must expose ON/OFF toggles that invoke their configured `startCommand`/`stopCommand`.
- The HAProxy row must keep the existing single-toggle process behavior.
- Toggle state must reflect latest observed health (`up` => ON, `down` => OFF); a toggle action must invoke start or stop immediately.
- The Admin Portal Command Center view must show the same service rows and toggles as the upstream Command Center view for the same config.

## Race-condition handling

- Start/stop actions may race with health polling; displayed state must converge to latest observed `healthUrl` result.
- Concurrent start/stop actions on the same service instance must be serialized by the command runner.
- Stopping a SubmissionManager instance may trigger leader failover; Command Center must continue reporting only instance health, not leadership.
- Stopping one delivery-tracking instance may overlap with traffic handled by other healthy delivery-tracking instances behind HAProxy; Command Center must report per-instance health only and must not infer aggregate delivery-tracking availability from one instance.

## Failure semantics

- If a start/stop command fails, the action must be reported as failed and health display must remain derived from `healthUrl`.
- If `healthUrl` is unreachable or non-2xx, the instance must be shown as down.
- Missing or empty `startCommand`/`stopCommand` on required delivery-tracking service entries is a config contract violation for this slice.
- Command Center or Admin Portal proxy failures must not mutate underlying service state; they affect only visibility/control UX.

## Concurrency guarantees

- Start/stop actions must be scoped to a single configured instance unless an operator-provided command intentionally targets more than one process.
- Health checks must remain per-instance and must not be aggregated into synthetic cross-instance health.
- Delivery-tracking instances must be independently toggleable; toggling one delivery-tracking instance must not implicitly toggle another instance.

## Observable acceptance criteria

Criterion 1: Command Center config includes one service entry per SubmissionManager instance and one service entry per delivery-tracking instance, each with one instance and configured `startCommand`/`stopCommand`.

Criterion 2: Command Center UI lists all configured SubmissionManager instances, delivery-tracking instances, and HAProxy row(s), and displays each instance `addr` exactly as configured.

Criterion 3: Each listed instance shows `up` when its `healthUrl` returns 2xx and `down` otherwise.

Criterion 4: ON/OFF toggle actions for delivery-tracking instances invoke configured start/stop commands in the same way as existing per-instance service toggles.

Criterion 5: Admin Portal `/command-center/ui` renders the same service rows and toggle affordances as direct Command Center `/ui` for the same upstream config.
