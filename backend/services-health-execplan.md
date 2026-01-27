# Add a services health console

This ExecPlan is a living document. The sections Progress, Surprises & Discoveries, Decision Log, and Outcomes & Retrospective must be kept up to date as work proceeds.

This plan follows `PLANS.md` from the repository root and must be maintained in accordance with it.

## Purpose / Big Picture

After this change, a user can open a dedicated services health console UI and see which gateways and supporting tools are up or down, which ports they are running on, and where their UIs can be accessed. The same console can start or stop configured services using explicit commands and input fields for required options like `-config` and `-addr`. This works without changing any existing gateway or provider behavior, and uses the existing HTML/HTMX + Go stack.

## Progress

- [x] (2026-01-26 11:30Z) Review existing UI/template and config parsing patterns to mirror current conventions.
- [x] (2026-01-26 11:30Z) Add a new services health command with config parsing, status checks, and start/stop actions.
- [x] (2026-01-26 11:31Z) Add HTML/HTMX templates for the health console UI.
- [x] (2026-01-26 11:31Z) Add a sample health config covering sms-gateway, push-gateway, grafana, prometheus, and haproxy.
- [x] (2026-01-26 11:32Z) Add tests for config parsing and port status checks.
- [x] (2026-01-26 11:32Z) Update README and add AGENTS.md for the new command.

## Surprises & Discoveries

- None yet.

## Decision Log

- Decision: Name the new command `services-health` and avoid any internal project labels in paths, endpoints, or UI text.
  Rationale: The name is internal and should not leak into user-facing surfaces.
  Date/Author: 2026-01-26 / Codex

- Decision: Determine service health by connecting to configured TCP ports, not by inspecting process lists.
  Rationale: Port reachability is portable and reflects actual availability.
  Date/Author: 2026-01-26 / Codex

- Decision: Start/stop actions are executed from explicit command arrays in the config with placeholder substitution (`{config}`, `{addr}`, `{port}`).
  Rationale: Keeps control explicit and allows different environments to define their own commands without code changes.
  Date/Author: 2026-01-26 / Codex

## Outcomes & Retrospective

Implemented a standalone services health console with TCP-based status checks, HTMX UI, and configurable start/stop commands. Added sample config covering gateways and tooling, tests for config parsing and status checks, and README/AGENTS updates.

## Context and Orientation

The repository has two gateway entrypoints: `cmd/sms-gateway/main.go` and `cmd/push-gateway/main.go`. Both use HTML templates from `../ui/` and serve static assets from `../ui/static/`. Config files live in `conf/` and support full-line `#` comments; addresses are passed via `-addr` flags. Observability tools (Prometheus, Grafana, HAProxy) are configured in `conf/` and accessed via their own ports.

The health console must live outside the gateways/providers and must not alter their behavior. The UI stack is HTML + HTMX with server-rendered fragments only.

## Plan of Work

Create a new command under `cmd/services-health/` that serves an HTML/HTMX UI at `/ui`. The command reads a lightweight JSON config (`conf/docker/services_health.json`) with `#` comment support, listing services and their instances, including gateways and supporting tools (Grafana, Prometheus, HAProxy). Each instance includes an address and optional UI URL. The server checks each address by attempting a short HTTP GET to `healthUrl` and renders an up/down status. Start/stop commands are defined in config and executed on demand via POST actions, with placeholder replacement and input fields for required options. UI templates live under `../ui/` and mirror existing styling; no JavaScript beyond HTMX.

## Concrete Steps

1) Create `cmd/services-health/main.go` with:
   - flag parsing (`-config`, `-addr`).
   - config loading with `#` full-line comment stripping.
   - status checks using TCP connect.
   - handlers for `/ui`, `/ui/services`, `/ui/services/start`, `/ui/services/stop`.

2) Add templates:
   - `../ui/health_overview.tmpl` for the page shell.
   - `../ui/health_services.tmpl` for the HTMX-updated service list and forms.

3) Add `conf/docker/services_health.json` with services:
   - sms-gateway, push-gateway, prometheus, grafana, haproxy.

4) Add tests under `cmd/services-health/main_test.go`:
   - config parsing (valid and comment stripping).
   - port status checks using a temporary TCP listener.

5) Update README with a "Services health console" section and add `cmd/services-health/AGENTS.md`.

## Validation and Acceptance

From repo root:

  go run ./cmd/services-health -config conf/docker/services_health.json -addr :8070

Open `http://localhost:8070/ui` and verify:
- Services show up/down status with ports and UI links (when configured).
- Start action runs the configured command (use a gateway example).
- Stop action uses the configured stop command and the status changes to down.

## Idempotence and Recovery

Status checks are read-only and safe to repeat. Start/stop actions run explicit commands; if a command fails, the UI returns the error without changing status. Running the health console multiple times does not mutate other services unless start/stop actions are invoked.

## Artifacts and Notes

The health config is JSON with optional full-line `#` comments. Commands are arrays of strings. Placeholders supported: `{config}`, `{addr}`, `{port}`. Relative paths are resolved from the health console’s working directory.

## Interfaces and Dependencies

No new dependencies are added. All code uses the Go standard library and existing UI assets.

Key types to define in `cmd/services-health/main.go`:
- `fileConfig`, `serviceConfig`, `serviceInstance` for config parsing.
- `serviceView` and `instanceView` for rendering templates.

---

Change log: Initial plan drafted to include supporting tools (Grafana, Prometheus, HAProxy) and neutral naming. (2026-01-26 / Codex)
Change log: Marked progress complete and recorded outcomes after implementation. (2026-01-26 / Codex)
