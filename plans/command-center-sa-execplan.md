# Command Center: SubmissionManager + HAProxy services

This execplan is a living document. The sections Progress, Surprises & Discoveries, Decision Log, and Outcomes & Retrospective must be kept up to date as work proceeds.

This plan follows `backend/PLANS.md` from the repository root and must be maintained in accordance with it.

## Purpose / Big Picture

After this change, operators can open the Command Center and see multiple SubmissionManager instances and the HAProxy instance in the services list, with each instance showing its configured address exactly as written in config. The same list and controls remain visible through the Admin Portal’s Command Center proxy. Start/stop toggles keep their existing behavior and use the configured commands. The visible proof is that the Services table shows Submission Manager entries, HAProxy entries, and each instance chip includes the raw `addr` string rather than a derived port.

## Progress

- [x] (2026-02-04 10:17Z) Read `specs/command-center-sa.md` and existing services health config/UI implementation.
- [x] (2026-02-04 10:18Z) Update the Command Center services UI to display each instance’s `addr` verbatim.
- [x] (2026-02-04 11:22Z) Remove the redundant actions column and align instance controls with their chips in the Instances column.
- [x] (2026-02-04 10:18Z) Confirm `backend/conf/docker/services_health.json` already declares SubmissionManager and HAProxy services as required; no config changes needed.
- [x] (2026-02-04 10:18Z) Run `go test ./cmd/services-health`.
- [x] (2026-02-04 11:35Z) Update `CHANGELOG.md` with the Command Center services list changes.
- [ ] Validate the Command Center and Admin Portal views show the same list and controls.
- [ ] Record outcomes and any surprises in this plan.

## Surprises & Discoveries

- None yet.

## Decision Log

- Decision: Display the configured `addr` directly in the instance chip, replacing the derived port display, to satisfy the “verbatim addr” requirement without adding new UI elements.
  Rationale: The spec requires showing the configured `addr` string; replacing the port display is the smallest change that guarantees compliance and avoids redundant or derived output.
  Date/Author: 2026-02-04 / Codex

- Decision: Move start/stop controls into the Instances column and remove the dedicated actions column, so each instance row includes its toggle without repeating labels.
  Rationale: The layout tweak removes unnecessary repetition and avoids component-name wrapping while keeping controls visible.
  Date/Author: 2026-02-04 / Codex

## Outcomes & Retrospective

Updated the services list UI to render instance `addr` values verbatim. The existing Docker services health config already includes SubmissionManager and HAProxy entries matching the spec. Automated tests for `cmd/services-health` pass. Manual UI validation in the Command Center and Admin Portal remains to be done.
Updated `CHANGELOG.md` to record the Command Center services list changes under 2026-02-04.

## Context and Orientation

The Command Center is implemented by `backend/cmd/services-health` and renders HTML/HTMX fragments from templates in `ui/`. The services list comes from `backend/conf/docker/services_health.json`, which follows the schema in `specs/services-health.md`. Admin Portal proxies the Command Center UI at `/command-center/ui` using `backend/cmd/admin-portal` and the `commandCenterUrl` in `backend/conf/docker/admin_portal_docker.json`. The services list UI is defined in `ui/health_services.tmpl`, which currently renders instance name, a derived port, and status.

In this repo, a “service” is a top-level grouping in the services health config, and an “instance” is a single address with a health URL. Start/stop commands are executed by the services-health command from config and are the only control-plane actions exposed.

## Plan of Work

First, change the services list template so the instance chip displays the configured `addr` string verbatim instead of the derived port. Next, remove the dedicated actions column and place the start/stop toggles next to each instance chip (and a single “Process” toggle for single-toggle services) inside the Instances column. This keeps controls visible without repeating instance labels. Second, verify that `backend/conf/docker/services_health.json` already contains one service per SubmissionManager instance (each with one instance) and a HAProxy service with multiple instances and `singleToggle` + `toggleInstance`. Update the config only if anything is missing. Finally, validate by running the services health console and confirming the Services table shows SubmissionManager and HAProxy entries with the raw `addr` strings, and that the Admin Portal still renders the same list via the Command Center proxy.

## Concrete Steps

1) Edit `ui/health_services.tmpl` to replace the derived port display with the instance `addr` string and to move action controls into the Instances column, removing the extra actions column.

2) Update `ui/static/ui.css` so the first column does not wrap, and the Instances column layout aligns chips with their toggles.

3) Review `backend/conf/docker/services_health.json` and ensure:
   - Services `submission-manager-<n>` exist for each instance, each with one instance and matching names/labels.
   - Service `haproxy` exists with multiple instances, `singleToggle: true`, and `toggleInstance` set.
   - Each instance includes `name`, `addr`, and `healthUrl`.
   If any item is missing, update the config accordingly.

4) Run the services health console from `backend/` and spot-check the UI.

## Validation and Acceptance

From `backend/` run:

  go run ./cmd/services-health -config conf/docker/services_health.json -addr :8070

Open `http://localhost:8070/ui` and verify:

- The Services table includes Submission Manager and HAProxy entries.
- Each instance chip shows the configured `addr` string exactly as it appears in the config.
- The existing ON/OFF toggles are present for each instance (or a single toggle for HAProxy).
- The Services table uses two columns, with actions aligned beside instance chips and no wrapping of the component labels at typical viewport widths.

Then, if the Admin Portal is running, open `http://localhost:8090/command-center/ui` and verify the same list and controls appear (with the theme toggle hidden because of `embed=1`).

## Idempotence and Recovery

The template change is safe to apply multiple times. If the UI looks wrong, revert the template change to restore the previous port display. Config changes are static JSON edits and can be reverted by restoring the previous file contents.

## Artifacts and Notes

If you need a quick visual check, look for instance chips that now include values like `:18082` or `localhost:18082` exactly as written in `backend/conf/docker/services_health.json`.

Test run (from `backend/`):

    go test ./cmd/services-health
    ok  	gateway/cmd/services-health	0.937s

Plan change note: Updated the plan to reflect the UI layout change that removes the actions column and aligns controls with instance chips, along with the associated CSS adjustments, because the user requested a cleaner layout without wrapping or repetition. (2026-02-04 / Codex)
Plan change note: Recorded the changelog update as part of the execution progress and outcomes. (2026-02-04 / Codex)

## Interfaces and Dependencies

No new dependencies are added. The change is limited to the HTML template in `ui/health_services.tmpl` and, if needed, the JSON config in `backend/conf/docker/services_health.json`.
