# Admin portal
COMPLETED

## Purpose

The admin portal is a thin HTML shell that proxies the SMS gateway console, push gateway console, HAProxy status, and the services health console under a unified Setu UI.

## Config

Config file path defaults to `conf/admin_portal.json`.

Fields:

- `title` (optional) - page title used in the shell
- `smsGatewayUrl` (optional) - base URL for SMS gateway UI and API
- `pushGatewayUrl` (optional) - base URL for push gateway UI and API
- `submissionManagerUrl` (optional) - base URL for SubmissionManager
- `submissionManagerDashboardUrl` (optional) - Grafana dashboard URL for SubmissionManager metrics
- `smsSubmissionTarget` (optional) - submissionTarget used for test SMS when routing through SubmissionManager
- `pushSubmissionTarget` (optional) - submissionTarget used for test push when routing through SubmissionManager
- `commandCenterUrl` (optional) - base URL for the services health console
- `haproxyStatsUrl` (optional) - HAProxy CSV stats endpoint (`/stats;csv`)

Empty URLs hide the corresponding navigation entry. HAProxy has no top-nav entry; `/haproxy` remains available when configured.

## HTTP endpoints

- `GET /ui` - redirects to `/command-center/ui`
- `GET /dashboards` - dashboards page (links to gateway metrics pages and the SubmissionManager dashboard page)
- `GET /dashboards/submission-manager` - embedded SubmissionManager Grafana dashboard
- `GET /haproxy` - HAProxy status view
- `GET /troubleshoot` - troubleshoot page with intent history panel
- `POST /troubleshoot/history` - proxies form data to SubmissionManager `/ui/history`
- `GET /sms/ui/*` - proxied SMS gateway UI
- `GET /push/ui/*` - proxied push gateway UI
- `GET /command-center/ui/*` - proxied services health UI
- `POST /sms/send` - submits to SubmissionManager when configured; otherwise proxied SMS send API
- `GET /sms/status?intentId=...` - queries SubmissionManager for current intent status when configured
- `POST /push/send` - submits to SubmissionManager when configured; otherwise proxied push send API
- `GET /push/status?intentId=...` - queries SubmissionManager for current intent status when configured
- `GET /sms/ui/troubleshoot` - portal troubleshoot page with intent history panel
- `POST /sms/ui/troubleshoot/history` - proxies form data to SubmissionManager `/ui/history` for authoritative intent history
- `GET /push/ui/troubleshoot` - portal troubleshoot page with intent history panel
- `POST /push/ui/troubleshoot/history` - proxies form data to SubmissionManager `/ui/history` for authoritative intent history
- `GET /healthz`, `GET /readyz`

## Proxy behavior

- UI requests are proxied with `HX-Request: true` to force fragment responses.
- UI responses with `Content-Type: text/html` are rewritten to prefix `/ui` links with the portal path (for example, `/sms/ui`).
- When proxying the Command Center UI, `embed=1` is added to the upstream query string and the theme toggle is stripped from the HTML.
- When proxying the SMS and push gateway UIs, `embed=1` is added so their internal navigation is hidden inside the portal.
- `/sms/send` is routed to SubmissionManager when `submissionManagerUrl` and `smsSubmissionTarget` are set; otherwise it proxies to the SMS gateway.
- `/push/send` is routed to SubmissionManager when `submissionManagerUrl` and `pushSubmissionTarget` are set; otherwise it proxies to the push gateway.
- When routing to SubmissionManager, the portal forwards an optional `waitSeconds` form value as the `waitSeconds` query parameter on `POST /v1/intents`.
- The portal exposes a troubleshoot page with intent history sourced from SubmissionManager persistence.

## HAProxy view

HAProxy status is read from the CSV stats endpoint. The portal parses frontends and backends from the CSV and renders:

- Frontend status, session count, and last change
- Backend status, number of servers up, and total servers

## UI shell

Non-HTMX responses are wrapped in a portal shell that includes:

- `portal_topbar.tmpl`
- static assets from `/ui/static/`
- HTMX and theme scripts

The top navigation is limited to: Command Center, Test SMS, Test Push, Dashboards, and Troubleshoot. Entries appear only when the corresponding config is set (for example, Troubleshoot requires `submissionManagerUrl`). The Test SMS and Test Push links go directly to the send forms (`/sms/ui/send`, `/push/ui/send`).

The Dashboards page presents three links: Submission Manager, SMS Gateway, and Push Gateway. Gateway dashboards point to `/sms/ui/metrics` and `/push/ui/metrics`. The SubmissionManager dashboard is embedded at `/dashboards/submission-manager` using `submissionManagerDashboardUrl`.
