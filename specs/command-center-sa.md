# Command Center: SubmissionManager and HAProxy services (draft)

## Purpose

Ensure the Command Center lists multiple SubmissionManager instances and the HAProxy instance, and allows operators to start/stop each from the UI. The same view must be visible in the Admin Portal via the Command Center proxy.

## Scope and non-goals

In scope:

- Command Center configuration for multiple SubmissionManager instances.
- Command Center configuration for the HAProxy instance.
- Start/stop actions for each instance via the Command Center UI.
- Health visibility for each instance in the Command Center UI.
- Visibility of the same Command Center view in the Admin Portal.

Out of scope:

- Changes to SubmissionManager execution semantics or leader lease behavior.
- Changes to HAProxy configuration or routing behavior.
- New Admin Portal navigation or layout changes.
- New health endpoints; the Command Center uses configured health URLs.

## Invariants

- Command Center health state is derived solely from the configured `healthUrl` for each instance.
- Start/stop commands are the only control plane actions exposed by the Command Center.
- Instance identity and display are defined by the Command Center config, not discovered dynamically.

## Configuration

This spec relies on the existing Command Center schema in `specs/services-health.md`. No schema changes are required.

Required service entries:

- A service entry for SubmissionManager with one instance per SubmissionManager process.
- A service entry for HAProxy with exactly one instance.

Each instance must set:

- `name` (unique within the service)
- `addr` (used for display and command substitution)
- `healthUrl` (returns 2xx when the instance is healthy)

Each service must define `startCommand` and `stopCommand` so the Command Center can perform stop/start actions. Commands should target a specific instance using the supported placeholders `{config}`, `{addr}`, and `{port}`.

Example (illustrative only):

```json
{
  "services": [
    {
      "id": "submission-manager",
      "label": "Submission Manager",
      "instances": [
        {"name": "sm-01", "addr": "sm-01:8082", "healthUrl": "http://sm-01:8082/healthz"},
        {"name": "sm-02", "addr": "sm-02:8082", "healthUrl": "http://sm-02:8082/healthz"}
      ],
      "startCommand": ["./bin/service-control", "start", "{addr}"],
      "stopCommand": ["./bin/service-control", "stop", "{addr}"]
    },
    {
      "id": "haproxy",
      "label": "HAProxy",
      "instances": [
        {"name": "haproxy-01", "addr": "haproxy:8404", "healthUrl": "http://haproxy:8404/healthz"}
      ],
      "startCommand": ["./bin/service-control", "start", "{addr}"],
      "stopCommand": ["./bin/service-control", "stop", "{addr}"]
    }
  ]
}
```

## UI behavior

- The Command Center UI lists all SubmissionManager instances and the HAProxy instance.
- Each instance displays its current health status based on `healthUrl`.
- Start and stop controls are available for each instance when `startCommand` and `stopCommand` are configured.
- The Admin Portal Command Center view shows the same instance list and controls.

## Race-condition handling

- Start/stop actions may race with health polling. The UI must reflect the latest observed health state from `healthUrl`.
- Concurrent start/stop actions on the same instance are serialized by the command runner; the resulting health state is authoritative.
- Stopping a SubmissionManager instance can trigger leader failover; Command Center only reflects instance health, not leadership.

## Failure semantics

- If a start/stop command fails, the action is reported as failed and the instance health remains based on `healthUrl`.
- If `healthUrl` is unreachable or returns non-2xx, the instance is shown as down.
- Command Center failures do not change the underlying service state; they only affect visibility and control.

## Concurrency guarantees

- Start/stop actions are scoped to a single instance; no action affects other instances unless defined by the operator’s command.
- Health checks are per-instance and do not aggregate across services.

## Observable acceptance criteria

- The Command Center UI lists all configured SubmissionManager instances and the HAProxy instance.
- Each instance shows `up` when its `healthUrl` returns 2xx and `down` otherwise.
- Start and stop actions are available for each instance and invoke the configured commands.
- The Admin Portal’s Command Center view shows the same list and allows the same actions.
