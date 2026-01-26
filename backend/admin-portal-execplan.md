# Unify Consoles In An Admin Portal

This ExecPlan is a living document. The sections `Progress`, `Surprises & Discoveries`, `Decision Log`, and `Outcomes & Retrospective` must be kept up to date as work proceeds.

This plan is governed by `backend/PLANS.md` and must be maintained in accordance with it.

## Purpose / Big Picture

Operators should be able to open a single admin portal and navigate between the SMS gateway console, the push gateway console, the HAProxy status view, and the Command Center without changing the individual services. The portal will share the existing Setu UI styling and provide a dark/light theme toggle so all embedded consoles look consistent.

## Progress

- [x] (2026-01-26 13:10Z) Add portal config and templates for overview, topbar navigation, and HAProxy stats.
- [x] (2026-01-26 13:10Z) Implement `cmd/admin-portal` with proxying for gateway/command-center UI fragments and HAProxy CSV rendering.
- [x] (2026-01-26 13:10Z) Update Command Center template + handler to support embed mode (hide the theme toggle).
- [x] (2026-01-26 13:10Z) Add minimal tests for HTML path rewriting and HAProxy CSV parsing.
- [x] (2026-01-26 13:11Z) Run `go test ./cmd/admin-portal` after adding portal tests.
- [x] (2026-01-26 13:12Z) Run `go test ./cmd/services-health` after updating the embed toggle behavior.
- [x] (2026-01-26 14:00Z) Re-run `go test ./cmd/admin-portal` after stripping embedded theme toggle.
- [x] (2026-01-26 14:26Z) Force HX-Request on proxied console UI to avoid duplicate topbars.
- [ ] (2026-01-26 12:52Z) Validate via `go run` and manual UI checks for navigation, send/troubleshoot forms, HAProxy stats, and theme toggle.

## Surprises & Discoveries

None yet.

## Decision Log

- Decision: Force HX-Request on all proxied console UI requests so upstreams return fragments only.
  Rationale: Prevents upstream shells from nesting inside the portal shell, eliminating duplicate topbars.
  Date/Author: 2026-01-26 / Codex

- Decision: Proxy HTML fragments from existing consoles and rewrite only `"/ui` attribute prefixes to keep console navigation working under portal paths.
  Rationale: Keeps the gateway/command-center logic intact and avoids duplicating their core behavior; still allows a unified portal shell and nav.
  Date/Author: 2026-01-26 / Codex

- Decision: Add an `embed=1` query option to Command Center to hide its theme toggle when rendered inside the portal.
  Rationale: Avoids duplicate `#theme-toggle` elements and keeps the portal’s single toggle authoritative.
  Date/Author: 2026-01-26 / Codex

- Decision: Render HAProxy data from `/stats;csv` with a lightweight CSV parser and minimal tables.
  Rationale: Keeps styling consistent with Setu CSS and avoids embedding HAProxy’s own HTML interface.
  Date/Author: 2026-01-26 / Codex

- Decision: Use full page loads for portal-level navigation while keeping embedded console navigation on HTMX.
  Rationale: The topbar active state lives in the shell, so full page loads keep it accurate without extra client logic; console-level nav stays within the active section.
  Date/Author: 2026-01-26 / Codex

## Outcomes & Retrospective

Not started.

## Context and Orientation

The existing gateway consoles live in `backend/cmd/sms-gateway/main.go` and `backend/cmd/push-gateway/main.go`. They serve HTML fragments from templates in `ui/` and return fragments when the `HX-Request: true` header is present. The Command Center lives in `backend/cmd/services-health/main.go` and uses `ui/health_overview.tmpl` plus `ui/health_services.tmpl`. All UI templates are under `ui/`, and static assets are under `ui/static/`.

The portal will be a new command under `backend/cmd/admin-portal`. It will serve a Setu-branded shell, a topbar navigation, and then proxy or render content into the `#ui-root` container. The portal will not implement gateway logic; it will only forward UI requests to the configured services and render the HAProxy CSV summary.

## Plan of Work

Add a new config file `backend/conf/admin_portal.json` with URLs for the SMS gateway, push gateway, Command Center, and HAProxy stats CSV. Implement a comment-stripping JSON loader similar to the Command Center so `#` full-line comments are permitted.

Create portal templates in `ui/` for a topbar navigation fragment, a portal overview page, a HAProxy status page, and a small error fragment. The overview will show cards that link to the portal paths for each console. The topbar will include the portal navigation links and the theme toggle button.

Implement `backend/cmd/admin-portal/main.go` as a standalone server. It will serve static assets from `ui/static`, render the portal overview, proxy UI fragments for `/sms/ui`, `/push/ui`, and `/command-center/ui` paths, and proxy API calls for `/sms/send` and `/push/send`. It will also fetch HAProxy CSV stats at `/haproxy` and render them using the shared CSS. The proxy will rewrite `"/ui` and `'/ui` attribute prefixes in the HTML fragments so internal console navigation resolves under the portal paths.

Update `ui/health_overview.tmpl` and `backend/cmd/services-health/main.go` so the Command Center can hide its theme toggle when `embed=1` is provided in the query string. The default behavior remains unchanged.

Add small tests in `backend/cmd/admin-portal/main_test.go` for HTML path rewriting and HAProxy CSV parsing so the portal’s critical transformations remain stable.

## Concrete Steps

1) Add `backend/conf/admin_portal.json` with example URLs. Use full-line `#` comments for guidance.

2) Add new portal templates in `ui/`:
   - `ui/portal_topbar.tmpl`
   - `ui/portal_overview.tmpl`
   - `ui/portal_haproxy.tmpl`
   - `ui/portal_error.tmpl`

3) Update `ui/static/ui.css` with portal-specific topbar styles and active nav highlighting.

4) Implement `backend/cmd/admin-portal/main.go` with handlers and proxy logic.

5) Update `ui/health_overview.tmpl` and `backend/cmd/services-health/main.go` to support `embed=1`.

6) Add tests under `backend/cmd/admin-portal/main_test.go` for `rewriteUIPaths` and `parseHAProxyCSV` behavior.

7) Run the portal locally from `backend/`:

   go run ./cmd/admin-portal -config conf/admin_portal.json -addr :8090

8) Validate by opening:

   http://localhost:8090/ui
   http://localhost:8090/sms/ui
   http://localhost:8090/push/ui
   http://localhost:8090/command-center/ui
   http://localhost:8090/haproxy

Verify that navigation and forms work and that the theme toggle changes the page appearance.

## Validation and Acceptance

Start the portal and confirm that the overview page renders with Setu styling and topbar navigation. Clicking SMS or Push in the topbar should load the respective console and allow navigation between Overview, Send, Troubleshoot, and Metrics using the embedded console nav. Submitting a Send form should return a gateway response fragment without errors. The Command Center view should render without a theme toggle button. The HAProxy page should show frontends/backends with status badges. Toggling the theme should update the portal and embedded consoles.

Run `go test ./cmd/admin-portal` and expect the new tests to pass.

## Idempotence and Recovery

Edits are additive and can be rerun safely. If the portal fails to start, re-run with a valid config URL and verify that the upstream services are reachable. Reverting the portal is safe by removing `cmd/admin-portal` and the portal templates if needed.

## Artifacts and Notes

Expected terminal output when starting the portal:

  listening on :8090 configPath="conf/admin_portal.json"

If a proxy target is unavailable, the portal should show an error panel indicating the unreachable upstream.

## Interfaces and Dependencies

The portal uses only Go standard library packages: `net/http`, `net/url`, `encoding/csv`, `encoding/json`, `html/template`, and helpers. No new dependencies are added.

Define in `backend/cmd/admin-portal/main.go`:

- `type fileConfig struct` with fields: `Title`, `SMSGatewayURL`, `PushGatewayURL`, `CommandCenterURL`, `HAProxyStatsURL`.
- `type portalServer struct` containing config, templates, static dir, and HTTP client.
- `func rewriteUIPaths(html []byte, prefix string) []byte`.
- `func parseHAProxyCSV(data []byte) (frontends []haproxyRow, backends []haproxyBackend, err error)` (names can vary but must be explicit).
- HTTP handlers for `/ui`, `/sms/ui*`, `/push/ui*`, `/command-center/ui*`, `/sms/send`, `/push/send`, and `/haproxy`.


## Plan Update Notes

2026-01-26: Marked completed progress items, added a decision about portal navigation using full page loads to keep topbar active state accurate.
2026-01-26: Recorded the test run for the new admin-portal package.
2026-01-26: Added a progress entry for the services-health test run after embed changes.
2026-01-26: Added stripping of embedded theme toggle and updated admin-portal tests.
2026-01-26: Forced HX-Request when proxying UI fragments to prevent nested topbars.

