# Delivery Tracking V1 Observability Surfacing Implementation

This execplan is a living document. The sections `Progress`, `Surprises & Discoveries`, `Decision Log`, and `Outcomes & Retrospective` must be kept up to date as work proceeds.

This document is maintained in accordance with `backend/PLANS.md`.

EXECPLAN-READY

## Purpose / Big Picture

After this change, delivery-tracking observability will be visible end-to-end in the local stack: Prometheus will scrape delivery-tracking instances, Grafana will provide two dedicated dashboards (`Delivery Tracking Health` and `Delivery Tracking Activity`), and the admin portal dashboards page will expose links/routes for both dashboards.

A novice can verify the behavior by checking the Prometheus scrape jobs config, opening the two provisioned Grafana dashboards, and running admin-portal tests that validate the two dashboard routes and config fields.

## Progress

- [x] (2026-02-11 08:08Z) Read required governance and inputs (`AGENTS.md`, `agents/EXEC.md`, component AGENTS, `backend/PLANS.md`, updated observability/admin-portal specs, existing observability design notes).
- [x] (2026-02-11 08:14Z) Added `delivery-tracking` scrape jobs to `backend/conf/docker/prometheus_docker.yml` and `backend/conf/prometheus.yml` with direct instance targets and `/metrics` path.
- [x] (2026-02-11 08:19Z) Added Grafana dashboards `backend/conf/grafana/dashboards/delivery-tracking-overview.json` and `backend/conf/grafana/dashboards/delivery-tracking-activity.json` with fixed UIDs and exactly five top-level panels each.
- [x] (2026-02-11 08:22Z) Extended admin-portal config, routes, handlers, tests, and dashboard template for `deliveryTrackingActivityDashboardUrl` and `/dashboards/delivery-tracking-activity`.
- [x] (2026-02-11 08:23Z) Updated portal config samples and docs (`backend/conf/admin_portal.json`, `backend/conf/docker/admin_portal_docker.json`, `backend/cmd/admin-portal/README.md`, `backend/README.md`) for two delivery dashboard links.
- [x] (2026-02-11 08:24Z) Ran focused validation (`go test -count=1 ./cmd/admin-portal`, dashboard JSON parse checks, and panel-count/UID checks).

## Surprises & Discoveries

- Observation: Existing component guidance at `backend/conf/grafana/AGENTS.md` says "Two dashboards only: SMS and push", but repository state already includes `submission-manager-overview.json`.
  Evidence: `backend/conf/grafana/dashboards` contains `gateway-overview-sms.json`, `gateway-overview-push.json`, and `submission-manager-overview.json`.
- Observation: Admin-portal templates are loaded from repository `ui/`, not from `backend/ui/`.
  Evidence: `findUIDir()` in `backend/cmd/admin-portal/config.go` searches parent-level `ui`, and the dashboards template path is `ui/portal_dashboards.tmpl`.

## Decision Log

- Decision: Create a new execplan file for the surfacing delta instead of mutating the already-completed runtime observability execplan.
  Rationale: Keeps execution history clear: prior plan covered runtime metric/log emission; this plan covers Prometheus/Grafana/admin-portal surfacing added by the new SPEC diff.
  Date/Author: 2026-02-11 / Codex
- Decision: Use `/dashboards/delivery-tracking` for Health and `/dashboards/delivery-tracking-activity` for Activity while keeping both URLs independently optional in config.
  Rationale: Matches updated spec criteria and keeps route semantics explicit and non-aliased.
  Date/Author: 2026-02-11 / Codex
- Decision: Populate both delivery dashboard URLs in default and Docker admin-portal configs.
  Rationale: Ensures delivery dashboards are surfaced immediately in local runs without additional manual config.
  Date/Author: 2026-02-11 / Codex

## Outcomes & Retrospective

This plan’s scope is complete. Prometheus now has a dedicated `delivery-tracking` job in both Docker and retained non-Docker configs. Grafana now provisions two delivery-tracking dashboards with fixed UIDs (`delivery-tracking-overview`, `delivery-tracking-activity`) and five KPI panels each, aligned to the frozen spec.

Admin portal now supports two delivery dashboard links/routes via separate config fields: `deliveryTrackingDashboardUrl` (Health) and `deliveryTrackingActivityDashboardUrl` (Activity). The dashboard list and embed handlers render each independently, and tests cover both configured and not-configured behavior.

Focused validation passed without regressions in the admin-portal package.

## Context and Orientation

Delivery-tracking metrics are already emitted by the delivery runtime and exposed at `/metrics`. This plan adds the remaining operator surfacing layers.

Prometheus configs live in `backend/conf/docker/prometheus_docker.yml` (Docker Compose path) and `backend/conf/prometheus.yml` (retained non-Docker path). Grafana provisioning reads dashboard JSON files from `backend/conf/grafana/dashboards`.

Admin portal dashboard behavior lives in `backend/cmd/admin-portal`:

- dashboard route wiring is in `backend/cmd/admin-portal/main.go`
- dashboard link/handler logic is in `backend/cmd/admin-portal/dashboards.go`
- config schema and normalization are in `backend/cmd/admin-portal/types.go` and `backend/cmd/admin-portal/config.go`
- dashboard UI template is `ui/portal_dashboards.tmpl`
- tests are in `backend/cmd/admin-portal/main_test.go`

## Plan of Work

First, update Prometheus scrape configs to include a dedicated `delivery-tracking` job targeting both delivery-tracking instances on `/metrics`.

Second, add two Grafana dashboard JSON assets under `backend/conf/grafana/dashboards` with fixed UIDs and titles from spec. Queries will use only bounded low-cardinality labels already defined by delivery metrics. The Health dashboard will focus on trouble indicators; the Activity dashboard will focus on selected time-window totals and mix.

Third, extend admin-portal configuration and route handling with a second delivery dashboard URL (`deliveryTrackingActivityDashboardUrl`) and a new embed route (`/dashboards/delivery-tracking-activity`). Update dashboard list rendering so both delivery dashboard links appear independently when configured.

Fourth, update portal config examples and relevant docs so defaults point to the new dashboards and operator instructions remain accurate.

Finally, run focused tests for `cmd/admin-portal` and verify dashboard/config artifacts are present and valid JSON.

## Concrete Steps

From repository root (`/Users/rajeshk/.codex/worktrees/9db5/setu`):

1. Edit:
   - `backend/conf/docker/prometheus_docker.yml`
   - `backend/conf/prometheus.yml`
   - `backend/conf/grafana/dashboards/delivery-tracking-overview.json` (new)
   - `backend/conf/grafana/dashboards/delivery-tracking-activity.json` (new)
   - `backend/cmd/admin-portal/types.go`
   - `backend/cmd/admin-portal/config.go`
   - `backend/cmd/admin-portal/dashboards.go`
   - `backend/cmd/admin-portal/main.go`
   - `backend/cmd/admin-portal/main_test.go`
   - `ui/portal_dashboards.tmpl`
   - `backend/conf/docker/admin_portal_docker.json`
   - `backend/conf/admin_portal.json`
   - `backend/cmd/admin-portal/README.md`
   - `backend/README.md`

2. Run tests from `backend/`:
   - `go test -count=1 ./cmd/admin-portal`

3. Optional sanity check from repo root:
   - `jq empty backend/conf/grafana/dashboards/delivery-tracking-overview.json backend/conf/grafana/dashboards/delivery-tracking-activity.json`

## Validation and Acceptance

Acceptance is met when:

- Prometheus configs contain a `delivery-tracking` scrape job with expected targets and `/metrics` path.
- Grafana has two dashboard JSON files with UIDs `delivery-tracking-overview` and `delivery-tracking-activity`, each containing exactly five top-level panels aligned to spec KPIs.
- Admin portal accepts the new `deliveryTrackingActivityDashboardUrl` config field, exposes a new route `/dashboards/delivery-tracking-activity`, and renders independent links for health/activity dashboards when configured.
- `go test -count=1 ./cmd/admin-portal` passes.

## Idempotence and Recovery

All planned edits are additive and safe to reapply. If tests fail mid-run, rerun the same `go test` command after fixing the failing file. No schema migrations or destructive operations are involved.

## Artifacts and Notes

Validation evidence:

    $ (cd backend && go test -count=1 ./cmd/admin-portal)
    ok  	gateway/cmd/admin-portal	0.491s

    $ jq empty backend/conf/grafana/dashboards/delivery-tracking-overview.json backend/conf/grafana/dashboards/delivery-tracking-activity.json
    (no output, exit 0)

    $ jq '.uid, .title, (.panels|length)' backend/conf/grafana/dashboards/delivery-tracking-overview.json backend/conf/grafana/dashboards/delivery-tracking-activity.json
    "delivery-tracking-overview"
    "Delivery Tracking Health"
    5
    "delivery-tracking-activity"
    "Delivery Tracking Activity"
    5

## Interfaces and Dependencies

No new Go dependencies are required. Dashboard assets use existing Grafana JSON structure and existing Prometheus datasource UID `prometheus`.

---

Revision note (2026-02-11 / Codex): Completed implementation and validation steps for Prometheus scrape wiring, dual delivery dashboards, and admin-portal dual dashboard linkage; updated all living sections with concrete evidence.
