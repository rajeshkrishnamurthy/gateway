# Admin portal

## Purpose

The admin portal is a thin HTML shell that proxies the SMS gateway console, push gateway console, HAProxy status, and the services health console under a unified Setu UI.

## Config

Config file path defaults to `conf/admin_portal.json`.

Fields:

- `title` (optional) - page title used in the shell
- `smsGatewayUrl` (optional) - base URL for SMS gateway UI and API
- `pushGatewayUrl` (optional) - base URL for push gateway UI and API
- `commandCenterUrl` (optional) - base URL for the services health console
- `haproxyStatsUrl` (optional) - HAProxy CSV stats endpoint (`/stats;csv`)

Empty URLs hide the corresponding navigation entry.

## HTTP endpoints

- `GET /ui` - portal overview
- `GET /haproxy` - HAProxy status view
- `GET /sms/ui/*` - proxied SMS gateway UI
- `GET /push/ui/*` - proxied push gateway UI
- `GET /command-center/ui/*` - proxied services health UI
- `POST /sms/send` - proxied SMS send API
- `POST /push/send` - proxied push send API
- `GET /healthz`, `GET /readyz`

## Proxy behavior

- UI requests are proxied with `HX-Request: true` to force fragment responses.
- UI responses with `Content-Type: text/html` are rewritten to prefix `/ui` links with the portal path (for example, `/sms/ui`).
- When proxying the Command Center UI, `embed=1` is added to the upstream query string and the theme toggle is stripped from the HTML.
- The portal rewrites the SMS console label "Troubleshoot by ReferenceId" to "Troubleshoot" in proxied HTML.
- API requests (`/sms/send`, `/push/send`) are proxied directly to the configured upstream URL.

## HAProxy view

HAProxy status is read from the CSV stats endpoint. The portal parses frontends and backends from the CSV and renders:

- Frontend status, session count, and last change
- Backend status, number of servers up, and total servers

## UI shell

Non-HTMX responses are wrapped in a portal shell that includes:

- `portal_topbar.tmpl`
- static assets from `/ui/static/`
- HTMX and theme scripts
