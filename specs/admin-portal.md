# Admin portal

EXEC-READY

## Purpose / Big picture

The admin portal must remain a thin HTML shell and proxy for operational runtimes, while giving operators one Setu UI entry point for submission and delivery-tracking operations.

With delivery tracking introduced as a dedicated multi-instance runtime behind HAProxy, the portal must expose delivery read operations and a guarded test producer path without changing delivery domain semantics or route ownership.

## Scope

This spec defines:

- portal configuration for existing surfaces plus delivery-tracking integration
- portal routes and UI views for current delivery and delivery history reads
- portal-operated provider signal test production for non-production environments
- dashboard-link behavior for delivery observability
- boundary constraints that preserve dedicated delivery-tracking ownership

## Non-goals

This spec does not define delivery domain semantics, signal-correlation rules, delivery-status progression rules, or delivery-freshness rules. Those remain governed by delivery-tracking specs.

This spec does not define direct instance addressing, sticky routing, or HAProxy configuration syntax.

This spec does not introduce a SubmissionManager compatibility path for delivery-tracking routes.

## Normative references

Delivery terms and delivery behavior referenced by this spec inherit from:

- `specs/delivery-tracking/ubiquitous-language.md`
- `specs/delivery-tracking/delivery-tracking-v1.md`
- `specs/delivery-tracking/delivery-tracking-v1-service-topology.md`
- `specs/delivery-tracking/delivery-tracking-v1-get-retrieval.md`
- `specs/delivery-tracking/delivery-tracking-v1-webhook-ingestion.md`
- `specs/delivery-tracking/delivery-tracking-v1-observability.md`

## Config

Config file path defaults to `backend/conf/admin_portal.json`.

Fields:

- `title` (optional) - page title used in the shell
- `smsGatewayUrl` (optional) - base URL for SMS gateway UI and API
- `pushGatewayUrl` (optional) - base URL for push gateway UI and API
- `submissionManagerUrl` (optional) - base URL for SubmissionManager
- `submissionManagerDashboardUrl` (optional) - Grafana dashboard URL for SubmissionManager metrics
- `smsSubmissionTarget` (optional) - `submissionTarget` used for test SMS when routing through SubmissionManager
- `pushSubmissionTarget` (optional) - `submissionTarget` used for test push when routing through SubmissionManager
- `commandCenterUrl` (optional) - base URL for the services health console
- `haproxyStatsUrl` (optional) - HAProxy CSV stats endpoint (`/stats;csv`)
- `deliveryTrackingUrl` (optional) - base URL for delivery-tracking runtime behind HAProxy
- `deliveryTrackingDashboardUrl` (optional) - Grafana dashboard URL for delivery-tracking observability
- `portalEnvironment` (optional) - one of `dev`, `staging`, `prod`; defaults to `prod` when omitted
- `deliveryTestProducerEnabled` (optional) - enables operator-triggered provider signal test production only when `portalEnvironment` is not `prod`; defaults to `false`

`deliveryTrackingUrl` must target the HAProxy delivery-tracking frontend and must not target a specific delivery-tracking instance address.

Empty URLs must hide related navigation and dashboards entries. HAProxy has no top-nav entry; `/haproxy` remains available when configured.

## HTTP endpoints

Portal must expose these endpoints:

- `GET /ui` - redirects to `/command-center/ui`
- `GET /dashboards` - dashboards page
- `GET /dashboards/submission-manager` - embedded SubmissionManager dashboard when configured
- `GET /dashboards/delivery-tracking` - embedded delivery-tracking dashboard when configured
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
- `GET /sms/ui/troubleshoot` - SMS portal troubleshoot page
- `POST /sms/ui/troubleshoot/history` - proxies to SubmissionManager `/ui/history`
- `GET /push/ui/troubleshoot` - push portal troubleshoot page
- `POST /push/ui/troubleshoot/history` - proxies to SubmissionManager `/ui/history`
- `GET /delivery/ui/current` - delivery current view form and result panel
- `GET /delivery/ui/history` - delivery history view form and result panel
- `GET /delivery/current?intentId=...` - proxies to `GET /v1/intents/{intentId}/delivery` on delivery-tracking runtime
- `GET /delivery/history?intentId=...` - proxies to `GET /v1/intents/{intentId}/delivery/history` on delivery-tracking runtime
- `GET /delivery/ui/test-producer` - provider signal test-producer form (non-production only)
- `POST /delivery/test-producer` - proxies operator payload to `POST /v1/delivery/provider-signal-webhook` on delivery-tracking runtime (non-production only)
- `GET /healthz`, `GET /readyz`

## Delivery integration behavior

The portal must proxy delivery read operations as thin pass-through calls to delivery-tracking runtime. The portal must not synthesize `delivery status`, `delivery freshness`, or `delivery history` values.

Delivery read routes must never use SubmissionManager as fallback. If `deliveryTrackingUrl` is configured, the portal must call only delivery-tracking runtime routes through HAProxy. If `deliveryTrackingUrl` is missing, delivery routes must be unavailable.

The test producer flow must follow the existing operator pattern used by Test SMS/Test Push: operator submits a portal form, portal issues one upstream request, and the portal renders the upstream response. For delivery test production, form data must map to webhook payload fields (`intentId`, `providerDeliverySignal`, optional `providerObservedAt`, optional provider metadata) and must be sent to `POST /v1/delivery/provider-signal-webhook` via `deliveryTrackingUrl`.

The test producer flow must be guarded for non-production only:

- when `portalEnvironment=prod`, `GET /delivery/ui/test-producer` and `POST /delivery/test-producer` must return `403`
- when `deliveryTestProducerEnabled=false`, those routes must return `404`
- only when `deliveryTestProducerEnabled=true` and `portalEnvironment` is `dev` or `staging` may those routes be available
- when both guard conditions apply (`portalEnvironment=prod` and `deliveryTestProducerEnabled=false`), production guard must take precedence and routes must return `403`

## Proxy behavior

- UI requests are proxied with `HX-Request: true` to force fragment responses.
- UI responses with `Content-Type: text/html` are rewritten to prefix `/ui` links with portal path prefixes.
- Command Center UI proxying adds `embed=1` and strips embedded theme toggle UI.
- SMS and push UI proxying add `embed=1` so embedded pages hide nested navigation.
- `/sms/send` routes to SubmissionManager when `submissionManagerUrl` and `smsSubmissionTarget` are set; otherwise it proxies to SMS gateway.
- `/push/send` routes to SubmissionManager when `submissionManagerUrl` and `pushSubmissionTarget` are set; otherwise it proxies to push gateway.
- When routing test submission through SubmissionManager, optional `waitSeconds` must be forwarded as `waitSeconds` query on `POST /v1/intents`.
- Delivery routes (`/delivery/current`, `/delivery/history`, `/delivery/test-producer`) must route only through `deliveryTrackingUrl`; no SubmissionManager fallback is allowed.

## HAProxy view

HAProxy status is read from the CSV stats endpoint. The portal parses frontends/backends and renders frontend status/session count/last change and backend status/servers up/total servers.

## UI shell and navigation

Non-HTMX responses are wrapped in the portal shell (`portal_topbar.tmpl`, static assets, HTMX, theme scripts).

Top navigation must remain config-gated and must include delivery operations when configured:

- `Delivery Current` links to `/delivery/ui/current` when `deliveryTrackingUrl` is set
- `Delivery History` links to `/delivery/ui/history` when `deliveryTrackingUrl` is set
- `Delivery Test Producer` links to `/delivery/ui/test-producer` only when `deliveryTestProducerEnabled=true` and `portalEnvironment` is non-production

Dashboards page must present existing dashboard links and, when configured, a delivery-tracking dashboard link at `/dashboards/delivery-tracking`.

## Invariants

The portal must remain a thin proxy/shell and must not own delivery-tracking domain behavior.

Delivery route ownership is exclusive to delivery-tracking runtime behind HAProxy for portal delivery operations. SubmissionManager must remain submission-only for portal integration.

Portal delivery routing must not address delivery-tracking instances directly and must not require operator/client awareness of instance identity.

Non-production guardrails for test producer are mandatory and must not be bypassed by hidden query params or alternate portal routes.

## Race conditions and handling

Concurrent reads and webhook ingestion updates may interleave. The portal must return one upstream response per request and must not merge or compose multi-request snapshots.

Concurrent operator actions hitting different healthy delivery-tracking instances through HAProxy must preserve delivery-route functional equivalence; the portal must not pin to instances.

Concurrent test-producer submissions for the same `intentId` must result in separate upstream webhook submissions; the portal must not coalesce or deduplicate requests.

## Failure semantics

If `deliveryTrackingUrl` is unset, delivery routes and delivery navigation must be unavailable and direct calls must return `404`.

If delivery upstream calls fail, the portal must surface upstream failure as gateway/proxy failure and must not reroute the request to SubmissionManager.

If `intentId` is unknown for delivery read calls, the portal must preserve delivery runtime not-found semantics.

If production guardrails block test-producer access, the portal must return `403` without sending any upstream webhook request.

## Concurrency guarantees

Portal request handling for delivery routes must be stateless and concurrency-safe, with no shared mutable cache that can fabricate or reorder delivery-read payloads.

Repeated delivery read requests over unchanged upstream persisted state must yield equivalent payloads irrespective of which healthy delivery-tracking instance serves the request through HAProxy.

## Observable acceptance criteria

Criterion 1: with `deliveryTrackingUrl` configured, `/delivery/ui/current` and `/delivery/ui/history` are visible and functional in the portal UI.

Criterion 2: `GET /delivery/current?intentId=<id>` results in one upstream call to `GET /v1/intents/<id>/delivery` via `deliveryTrackingUrl`, with status code preserved.

Criterion 3: `GET /delivery/history?intentId=<id>` results in one upstream call to `GET /v1/intents/<id>/delivery/history` via `deliveryTrackingUrl`, with status code preserved.

Criterion 4: no delivery read route in the portal calls SubmissionManager, even when `submissionManagerUrl` is configured and delivery upstream is unavailable.

Criterion 5: when `deliveryTrackingDashboardUrl` is configured, `/dashboards` shows a delivery-tracking link and `/dashboards/delivery-tracking` renders embedded dashboard content.

Criterion 6: when `deliveryTrackingDashboardUrl` is configured, the linked dashboard surfaces delivery observability for `correlation result` and canonical delivery-status count summaries (`unknown`, `in_progress`, `delivered`, `failed`).

Criterion 7: when `portalEnvironment=prod`, both delivery test-producer routes return `403` and no upstream webhook request is emitted.

Criterion 8: when `portalEnvironment` is non-production and `deliveryTestProducerEnabled=true`, posting the test-producer form emits one upstream `POST /v1/delivery/provider-signal-webhook` request through `deliveryTrackingUrl`.

Criterion 9: portal delivery routes remain functional across delivery runtime instance failover behind HAProxy without changing portal config or route paths.

Criterion 10: portal logs/telemetry for delivery proxy calls identify only HAProxy target hosts configured by `deliveryTrackingUrl`, with no direct instance targets.

Criterion 11: when `portalEnvironment=prod` and `deliveryTestProducerEnabled=false` simultaneously, delivery test-producer routes return `403` (not `404`) and no upstream webhook request is emitted.
