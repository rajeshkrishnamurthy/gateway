# AGENTS.md — Grafana

## Scope
Provisioned Grafana assets for gateway observability.

## Dashboards
- Two dashboards only: SMS and push.
- Keep queries aligned with gateway metrics and low-cardinality labels.
- Preserve dashboard UIDs and URLs to avoid breaking links.

## Provisioning
- Provisioning must continue to point at local Prometheus (`http://localhost:9090`).
- Dashboards are loaded from `conf/grafana/dashboards`.
- Avoid extra datasources or plugins.

## UI linkage
- Gateway UI links here via `GRAFANA_DASHBOARD_URL`.
- Keep dashboard slugs stable: `gateway-overview-sms` and `gateway-overview-push`.
