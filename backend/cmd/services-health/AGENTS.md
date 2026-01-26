# AGENTS.md — services-health

## Scope
`cmd/services-health` is a standalone services health console. It monitors gateways and supporting tools and provides start/stop controls via explicit commands in config.

## Boundaries
- Do not depend on gateway code or providers.
- Use Go stdlib only; no new dependencies.
- Health checks are TCP reachability to configured addresses.

## Config
- Config file lives under `conf/services_health.json` and allows full-line `#` comments.
- Services, instances, and start/stop commands are defined in config.
- Placeholder tokens are `{config}`, `{addr}`, `{port}` and must remain explicit.

## UI
- HTML + HTMX only; no custom JavaScript beyond a minimal theme toggle.
- Templates live under `../ui/` and are fragments, not full documents.
- Provide guidance for relative paths near config inputs.
