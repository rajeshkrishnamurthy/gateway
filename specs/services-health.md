# Services health console (Command Center)
COMPLETED

## Purpose

The services health console is a small UI for checking container status and running start/stop commands defined in config. It is a host-run tool and does not depend on gateway code.

## Config

Default config path is `conf/docker/services_health.json`. Configs are JSON and allow full-line `#` comments only. Unknown fields are rejected.

### Schema

- `services[]`
  - `id` (string)
  - `label` (string)
  - `instances[]`
    - `name` (string)
    - `addr` (string)
    - `uiUrl` (string, optional)
    - `healthUrl` (string, required, http/https)
    - `configPath` (string, optional)
  - `startCommand` (array of strings, optional)
  - `stopCommand` (array of strings, optional)
  - `defaultConfigPath` (string, optional)
  - `singleToggle` (bool, optional)
  - `toggleInstance` (string, optional)

## Health checks

- Status is determined by HTTP GET to `healthUrl`.
- A `2xx` response is considered up; any error or non-2xx is down.
- Each check uses a 2 second timeout.

## Start and stop commands

- Commands are arrays of strings executed by the host.
- Placeholders are substituted before execution:
  - `{config}` - config path
  - `{addr}` - instance address
  - `{port}` - port extracted from `addr`
- Start runs the command, then polls `healthUrl` until it is up or a timeout occurs.
- Stop runs the command and polls until the service is down.
- Start output is captured and truncated to 400 characters for error messages.

## Config path handling

The config viewer is read-only and only allows files under `conf/` relative to the console working directory. Paths must be relative and must not point to directories.

## HTTP endpoints

- `GET /` redirects to `/ui`
- `GET /healthz`, `GET /readyz`
- `GET /ui` - overview
- `GET /ui/services` - HTMX fragment for services list
- `POST /ui/services/start` - start action
- `POST /ui/services/stop` - stop action
- `GET /ui/config` - render config file contents
- `GET /ui/config/clear` - clear config view
- `GET /static/*` - static assets

## UI behavior

- The theme toggle and header navigation are hidden when `embed=1` or `embed=true` is present in the query string.
- Status labels are `up` and `down`, with CSS classes `status-up` and `status-down`.
- The console does not render direct interface links; the UI focuses on health and start/stop actions.
