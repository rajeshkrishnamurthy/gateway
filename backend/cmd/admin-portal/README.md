# Admin Portal

This command provides a single Setu-branded portal that links to the SMS gateway console, push gateway console, HAProxy status, and the Command Center. It proxies the existing UIs; SMS test submits can route through SubmissionManager when configured.

## Run (Docker-only MVP)

From the repo root:

```
docker compose up
```

Then open `http://localhost:8090/ui`.

If you want the Command Center embedded in the portal while running Docker Compose, start it on the host with:

```
go run ./cmd/services-health -config conf/docker/services_health.json -addr :8070
```

The Docker admin portal config points at `http://host.docker.internal:8070` for the Command Center URL.

## Configuration

Docker Compose uses `conf/docker/admin_portal_docker.json`.

`conf/admin_portal.json` is retained as a future non-Docker starting point but is not maintained for the MVP.

The config supports full-line `#` comments only.

- `title`: Page title used in the shell.
- `smsGatewayUrl`: Base URL for the SMS gateway UI (prefer the HAProxy SMS frontend).
- `pushGatewayUrl`: Base URL for the push gateway UI (prefer the HAProxy push frontend).
- `submissionManagerUrl`: Base URL for SubmissionManager (for test submits).
- `smsSubmissionTarget`: SubmissionTarget used when the portal submits test SMS via SubmissionManager.
- `pushSubmissionTarget`: SubmissionTarget used when the portal submits test push via SubmissionManager.
- `commandCenterUrl`: Base URL for the services health console.
- `haproxyStatsUrl`: HAProxy CSV stats endpoint, typically `http://localhost:8404/stats;csv`.

If a URL is empty, its navigation entry is hidden.

## Proxy Behavior

- UI requests are proxied as HTML fragments and embedded in the portal shell.
- The portal rewrites `"/ui` links to route within its own prefixes.
- `HX-Request` is forced on upstream UI calls to avoid nested shells.
- `/sms/send` submits to SubmissionManager when `submissionManagerUrl` and `smsSubmissionTarget` are set; otherwise it proxies to the SMS gateway.
- `/sms/status?intentId=...` queries SubmissionManager for the current intent status when configured.
- `/push/send` submits to SubmissionManager when `submissionManagerUrl` and `pushSubmissionTarget` are set; otherwise it proxies to the push gateway.
- `/push/status?intentId=...` queries SubmissionManager for the current intent status when configured.
- HAProxy status is rendered from CSV, not from the HTML stats page.

## Theme

The portal owns the theme toggle. Embedded UIs do not render their own toggle.
