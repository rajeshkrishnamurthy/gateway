# AGENTS.md — sms-gateway

## Scope
`cmd/sms-gateway` is the HTTP entrypoint for SMS submission. It owns route registration, flag parsing, and provider wiring.

## Boundaries
- Do not introduce Docker/Compose assumptions into gateway code; keep runtime wiring in config or ops tooling.
- Provider credentials are read from env in the provider switch; never from config files.

## Config + flags
- `-config` covers provider semantics/timeouts plus instance-agnostic gateway settings (e.g. `grafanaDashboardUrl`).
- Default config path is `conf/sms/config.json`.
- `-addr` is the instance bind address; do not move it into config.
- Full-line `#` comments are allowed in config files.

## Provider wiring
- One provider per process.
- Provider-specific env var names are hardcoded in the switch.
- No abstractions or indirection for credential lookup.

## Observability
- Preserve SMS logging and metrics semantics.
- Metrics UI link uses `grafanaDashboardUrl` from config; fall back to the default SMS dashboard URL.
