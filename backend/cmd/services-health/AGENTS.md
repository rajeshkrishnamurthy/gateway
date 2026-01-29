# AGENTS.md - services-health

## Scope
`cmd/services-health` is a standalone services health console. It monitors gateways and supporting tools and provides start/stop controls via explicit commands in config.

## Boundaries
- Do not depend on gateway code or providers.
- Use Go stdlib only; no new dependencies.
- Health checks are HTTP GET to each instance `healthUrl` (2xx = up).

## Config
- Config file is `conf/docker/services_health.json` and allows full-line `#` comments.
- For the MVP, the config is Docker Compose-only (start/stop commands call `docker compose`).
- Services, instances, and start/stop commands are defined in config.
- Each instance requires `healthUrl` for HTTP health checks.
- Placeholder tokens are `{config}`, `{addr}`, `{port}` and must remain explicit.
- Config viewing is read-only and restricted to files under `conf/`.

## UI
- HTML + HTMX only; no custom JavaScript beyond a minimal theme toggle.
- Templates live under `../ui/` and are fragments, not full documents.
- Provide guidance for relative paths near config inputs.
- `embed=1` hides the theme toggle for portal embedding.
