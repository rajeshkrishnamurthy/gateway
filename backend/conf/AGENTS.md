# AGENTS.md - conf

## Scope
Configuration and infra files for local/dev deployments: gateway configs, Prometheus, Grafana, and HAProxy.

## Gateway configs
- Docker Compose gateway configs live under `conf/docker/` (`config_docker.json`, `config_push_docker.json`).
- SMS gateway configs live under `conf/sms/` (`config.json`, `config_model.json`, `config_24x7.json`, `config_karix.json`, `config_infobip.json`).
- `conf/config_push.json` is retained as a future non-Docker push starting point but is not maintained for the MVP.
- Do not add bind ports or instance flags.
- Full-line `#` comments are allowed; inline comments are not.
- No secrets in config files; credentials come from environment variables.

## HAProxy
- `conf/docker/haproxy_docker.cfg` provides stable frontends (`:8080` SMS, `:8081` push) and routes to instance backends for Docker Compose.
- `haproxy.cfg` is retained for a future non-Docker path.
- Scale by adding or removing backend servers; gateways remain unchanged and unaware of peers.

## Prometheus
- `conf/docker/prometheus_docker.yml` scrapes gateway instances directly (not HAProxy) for Docker Compose.
- `prometheus.yml` is retained for a future non-Docker path.
- Keep separate jobs for SMS and push so dashboards can filter by job label.

## Grafana
- Provisioning lives under `conf/grafana/`; dashboards live in `conf/grafana/dashboards`.
- Keep dashboards minimal and aligned to existing gateway metrics.
