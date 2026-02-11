# Command Center: SubmissionManager + HAProxy + Delivery Tracking services

This execplan is a living document. The sections Progress, Surprises & Discoveries, Decision Log, and Outcomes & Retrospective must be kept up to date as work proceeds.

This plan follows `backend/PLANS.md` from the repository root and must be maintained in accordance with it.

## Purpose / Big Picture

After this change, operators can open the Command Center and see multiple SubmissionManager instances, delivery-tracking instances, and the HAProxy instance in the services list, with each instance showing its configured address exactly as written in config. The same list and controls remain visible through the Admin Portal’s Command Center proxy. Start/stop toggles keep their existing behavior and use the configured commands. The visible proof is that the Services table shows Submission Manager entries, delivery-tracking entries, and HAProxy entries with ON/OFF controls that match direct and proxied views.

## Progress

- [x] (2026-02-04 10:17Z) Read `specs/command-center-sa.md` and existing services health config/UI implementation.
- [x] (2026-02-04 10:18Z) Update the Command Center services UI to display each instance’s `addr` verbatim.
- [x] (2026-02-04 11:22Z) Remove the redundant actions column and align instance controls with their chips in the Instances column.
- [x] (2026-02-04 10:18Z) Confirm `backend/conf/docker/services_health.json` already declares SubmissionManager and HAProxy services as required; no config changes needed.
- [x] (2026-02-04 10:18Z) Run `go test ./cmd/services-health`.
- [x] (2026-02-04 11:35Z) Update `CHANGELOG.md` with the Command Center services list changes.
- [x] (2026-02-11 09:08Z) Scoped this EXEC pass to delivery-tracking visibility/start-stop parity delta; local `git diff -- specs/command-center-sa.md` was empty in the active worktree.
- [x] (2026-02-11 09:08Z) Add `delivery-tracking-1` and `delivery-tracking-2` service entries to `backend/conf/docker/services_health.json` with per-instance start/stop commands and health URLs.
- [x] (2026-02-11 09:09Z) Add focused `cmd/services-health` config test coverage to verify delivery-tracking rows are configured for per-instance ON/OFF behavior.
- [x] (2026-02-11 09:09Z) Add focused `cmd/admin-portal` proxy test coverage to verify `/command-center/ui` preserves delivery-tracking rows and ON/OFF form actions.
- [x] (2026-02-11 09:09Z) Run `go test ./cmd/services-health`.
- [x] (2026-02-11 09:09Z) Run `go test ./cmd/admin-portal`.
- [ ] Validate the Command Center and Admin Portal views show the same list and controls (completed: attempted `curl http://localhost:8070/ui` and `curl http://localhost:8090/command-center/ui`; remaining: run both servers and re-check UI content).
- [x] (2026-02-11 09:10Z) Record outcomes and surprises for this EXEC pass.

## Surprises & Discoveries

- Observation: The user-specified `/Users/rajeshk/.codex/worktrees/558f/setu` path is not present in this environment; only `/Users/rajeshk/.codex/worktrees/23d3/setu` is available.
  Evidence: `ls -la /Users/rajeshk/.codex/worktrees` returned `23d3` and `d3d9`; local `git diff -- specs/command-center-sa.md` was empty.

- Observation: Manual direct/proxied UI checks could not run because neither local endpoint was serving.
  Evidence: `curl -m 3 http://localhost:8070/ui` and `curl -m 3 http://localhost:8090/command-center/ui` both returned connection error 7.

## Decision Log

- Decision: Display the configured `addr` directly in the instance chip, replacing the derived port display, to satisfy the “verbatim addr” requirement without adding new UI elements.
  Rationale: The spec requires showing the configured `addr` string; replacing the port display is the smallest change that guarantees compliance and avoids redundant or derived output.
  Date/Author: 2026-02-04 / Codex

- Decision: Move start/stop controls into the Instances column and remove the dedicated actions column, so each instance row includes its toggle without repeating labels.
  Rationale: The layout tweak removes unnecessary repetition and avoids component-name wrapping while keeping controls visible.
  Date/Author: 2026-02-04 / Codex

- Decision: Implement the delivery-tracking delta through config wiring plus focused tests only, without changing services-health or admin-portal runtime logic.
  Rationale: Existing handlers and templates already support per-instance ON/OFF actions and proxy passthrough; missing behavior was the absence of delivery-tracking service definitions and explicit parity tests.
  Date/Author: 2026-02-11 / Codex

## Outcomes & Retrospective

Updated the services list UI to render instance `addr` values verbatim. Docker services health config now includes SubmissionManager, delivery-tracking, and HAProxy entries matching the latest EXEC scope for Command Center visibility and ON/OFF parity. Focused tests were added for `cmd/services-health` (config contract) and `cmd/admin-portal` (proxied row/toggle parity), and both package test commands passed. Manual UI validation in the Command Center and Admin Portal remains to be done once local services are running.
Updated `CHANGELOG.md` to record the earlier Command Center services list changes under 2026-02-04.

## Context and Orientation

The Command Center is implemented by `backend/cmd/services-health` and renders HTML/HTMX fragments from templates in `ui/`. The services list comes from `backend/conf/docker/services_health.json`, which follows the schema in `specs/services-health.md`. Admin Portal proxies the Command Center UI at `/command-center/ui` using `backend/cmd/admin-portal` and the `commandCenterUrl` in `backend/conf/docker/admin_portal_docker.json`. The services list UI is defined in `ui/health_services.tmpl`, which currently renders instance name, a derived port, and status.

In this repo, a “service” is a top-level grouping in the services health config, and an “instance” is a single address with a health URL. Start/stop commands are executed by the services-health command from config and are the only control-plane actions exposed.

## Plan of Work

For this EXEC pass, wire delivery-tracking into `backend/conf/docker/services_health.json` as two per-instance services (`delivery-tracking-1`, `delivery-tracking-2`) with explicit start/stop commands and health URLs that map to Docker Compose ports. Then add tests proving (a) config parity for per-instance ON/OFF prerequisites in `cmd/services-health` and (b) proxied HTML parity for `/command-center/ui` in `cmd/admin-portal`. Finally, run package tests and attempt direct/proxied UI checks.

## Concrete Steps

1) Edit `ui/health_services.tmpl` to replace the derived port display with the instance `addr` string and to move action controls into the Instances column, removing the extra actions column.

2) Update `ui/static/ui.css` so the first column does not wrap, and the Instances column layout aligns chips with their toggles.

3) Review `backend/conf/docker/services_health.json` and ensure:
   - Services `submission-manager-<n>` exist for each instance, each with one instance and matching names/labels.
   - Services `delivery-tracking-<n>` exist for each instance, each with one instance and matching names/labels.
   - Service `haproxy` exists with multiple instances, `singleToggle: true`, and `toggleInstance` set.
   - Each instance includes `name`, `addr`, and `healthUrl`.
   If any item is missing, update the config accordingly.

4) Add focused tests:
   - `backend/cmd/services-health/main_test.go` for Docker config entries and start/stop command parity.
   - `backend/cmd/admin-portal/main_test.go` for `/command-center/ui` row/toggle passthrough parity.

5) Run `go test ./cmd/services-health` and `go test ./cmd/admin-portal` from `backend/`.

6) Run the services health console from `backend/` and spot-check direct and proxied UI.

## Validation and Acceptance

From `backend/` run:

  go run ./cmd/services-health -config conf/docker/services_health.json -addr :8070

Open `http://localhost:8070/ui` and verify:

- The Services table includes Submission Manager, Delivery Tracking, and HAProxy entries.
- Each instance chip shows the configured `addr` string exactly as it appears in the config.
- The existing ON/OFF toggles are present for each instance (or a single toggle for HAProxy).
- The Services table uses two columns, with actions aligned beside instance chips and no wrapping of the component labels at typical viewport widths.

Then, if the Admin Portal is running, open `http://localhost:8090/command-center/ui` and verify the same list and controls appear (with the theme toggle hidden because of `embed=1`), including delivery-tracking rows with ON/OFF controls.

## Idempotence and Recovery

The template change is safe to apply multiple times. If the UI looks wrong, revert the template change to restore the previous port display. Config changes are static JSON edits and can be reverted by restoring the previous file contents.

## Artifacts and Notes

If you need a quick visual check, look for instance chips that include `:18082`, `:18083`, `:18084`, and `:18085` exactly as written in `backend/conf/docker/services_health.json`.

Test run (from `backend/`):

    go test ./cmd/services-health
    ok  	gateway/cmd/services-health	1.361s

    go test ./cmd/admin-portal
    ok  	gateway/cmd/admin-portal	0.506s

Manual check attempt:

    curl -m 3 http://localhost:8070/ui
    curl: (7) Failed to connect to localhost port 8070

    curl -m 3 http://localhost:8090/command-center/ui
    curl: (7) Failed to connect to localhost port 8090

Plan change note: Updated the plan to reflect the UI layout change that removes the actions column and aligns controls with instance chips, along with the associated CSS adjustments, because the user requested a cleaner layout without wrapping or repetition. (2026-02-04 / Codex)
Plan change note: Recorded the changelog update as part of the execution progress and outcomes. (2026-02-04 / Codex)
Plan change note: Added delivery-tracking config/test parity work for Command Center and Admin Portal proxy behavior in this EXEC pass, scoped to the user-requested delta. (2026-02-11 / Codex)

## Interfaces and Dependencies

No new dependencies are added. This EXEC pass changes Docker Command Center config (`backend/conf/docker/services_health.json`) and focused tests in `backend/cmd/services-health/main_test.go` and `backend/cmd/admin-portal/main_test.go`; no runtime handler/template logic changes were required.
