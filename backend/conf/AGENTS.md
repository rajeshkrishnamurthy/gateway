# AGENTS.md - conf

## Scope
Configuration and infra files for local/dev deployments: gateway configs, Prometheus, Grafana, and HAProxy.

## Gateway configs
- `config*_docker.json` are used by Docker Compose and define provider semantics, timeouts, and instance-agnostic gateway settings (e.g. `grafanaDashboardUrl`).
- Other `config*.json` files are retained as future non-Docker starting points but are not maintained for the MVP.
- Do not add bind ports or instance flags.
- Full-line `#` comments are allowed; inline comments are not.
- No secrets in config files; credentials come from environment variables.

## HAProxy
- `haproxy_docker.cfg` provides stable frontends (`:8080` SMS, `:8081` push) and routes to instance backends for Docker Compose.
- `haproxy.cfg` is retained for a future non-Docker path.
- Scale by adding or removing backend servers; gateways remain unchanged and unaware of peers.

## Prometheus
- `prometheus_docker.yml` scrapes gateway instances directly (not HAProxy) for Docker Compose.
- `prometheus.yml` is retained for a future non-Docker path.
- Keep separate jobs for SMS and push so dashboards can filter by job label.

## Grafana
- Provisioning lives under `conf/grafana/`; dashboards live in `conf/grafana/dashboards`.
- Keep dashboards minimal and aligned to existing gateway metrics.
