# AGENTS.md — sms-gateway

## Scope
`cmd/sms-gateway` is the HTTP entrypoint for SMS submission. It owns route registration, flag parsing, and provider wiring.

## Boundaries
- Keep all `net/http` usage and UI handlers in `main.go` here.
- Core gateway code must remain HTTP-agnostic.
- Provider credentials are read from env in the provider switch; never from `config.json`.

## Config + flags
- `-config` points to provider semantics and timeouts only.
- `-addr` is the instance bind address; do not move it into config.
- Full-line `#` comments are allowed in config files.

## Provider wiring
- One provider per process.
- Provider-specific env var names are hardcoded in the switch.
- No abstractions or indirection for credential lookup.

## Observability
- Preserve SMS logging and metrics semantics.
- Metrics UI link uses `GRAFANA_DASHBOARD_URL` when set; otherwise use the default SMS dashboard URL.
