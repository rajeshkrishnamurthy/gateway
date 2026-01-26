# AGENTS.md — conf

## Scope
Configuration and infra files for local/dev deployments: gateway configs, Prometheus, Grafana, and HAProxy.

## Gateway configs
- `config*.json` define provider semantics and timeouts only.
- Do not add bind ports or instance flags.
- Full-line `#` comments are allowed; inline comments are not.
- No secrets in config files; credentials come from environment variables.

## HAProxy
- `haproxy.cfg` provides stable frontends (`:8080` SMS, `:8081` push) and routes to instance backends.
- Scale by adding or removing backend servers; gateways remain unchanged and unaware of peers.

## Prometheus
- `prometheus.yml` scrapes gateway instances directly (not HAProxy).
- Keep separate jobs for SMS and push so dashboards can filter by job label.

## Grafana
- Provisioning lives under `conf/grafana/`; dashboards live in `conf/grafana/dashboards`.
- Keep dashboards minimal and aligned to existing gateway metrics.
