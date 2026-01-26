# Services Health Console

This command provides a small operations console for the gateways and supporting tools. It shows which container instances are reachable and lets you start or stop them using Docker Compose commands defined in config.

## Run (Docker-only MVP)

From `backend/`:

```
go run ./cmd/services-health -config conf/services_health.json -addr :8070
```

Then open `http://localhost:8070/ui`.

## Configuration

The config file is `conf/services_health.json` and supports full-line `#` comments only.

- Services and instances are declared in the file.
- Start and stop commands are explicit strings; placeholders `{config}`, `{addr}`, `{port}` are replaced at runtime.
- Relative paths are resolved from the current working directory.
- Config viewing is read-only and limited to files under `conf/`.
- Docker Compose must be available on the host PATH.

## Health Checks

Health checks are TCP connectivity only. A service is "up" when its port is listening.

## UI Notes

- The UI is HTML + HTMX fragments.
- The theme toggle uses local storage and is hidden when `embed=1` is present in the query string.
