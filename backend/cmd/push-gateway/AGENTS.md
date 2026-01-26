# AGENTS.md — push-gateway

## Scope
`cmd/push-gateway` is the HTTP entrypoint for push submission. It owns route registration, flag parsing, and provider wiring.

## Boundaries
- Keep all `net/http` usage and UI handlers in `main.go` here.
- Core push gateway code must remain HTTP-agnostic.
- Provider credentials are read from env in the provider switch; never from `config.json`.

## Config + flags
- `-config` points to provider semantics and timeouts only.
- `-addr` is the instance bind address; do not move it into config.
- Full-line `#` comments are allowed in config files.

## Credentials
- Read `PUSH_FCM_CREDENTIAL_JSON_PATH` (preferred) or `PUSH_FCM_BEARER_TOKEN` inside the provider switch.
- Do not log secrets.

## Observability
- Preserve logging and metrics semantics; only push nouns change.
- Metrics UI link uses `GRAFANA_DASHBOARD_URL` when set; otherwise use the default push dashboard URL.
