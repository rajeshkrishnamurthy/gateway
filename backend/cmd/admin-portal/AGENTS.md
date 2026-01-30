# AGENTS.md - admin-portal

## Scope
`cmd/admin-portal` is a thin UI proxy that unifies the SMS gateway console, push gateway console, HAProxy stats, and the Command Center under one Setu-branded shell.

## Boundaries
- Do not implement gateway logic here; proxy only.
- Use Go stdlib only; no new dependencies.
- Keep HTML rewriting minimal and explicit.
- Do not parse provider responses or add new metrics.
- Do not hardcode Docker/Compose-specific routing in portal logic; keep environment wiring in config files.
- Error handling: UI endpoints must render via `renderError`/fragments. API passthrough endpoints may use `http.Error` only for local proxy failures (bad URL/build/transport); otherwise preserve upstream status/body. Hybrid endpoints should render fragments for HTMX requests and return raw JSON for non-HTMX callers.

## Config
- Docker Compose uses `conf/docker/admin_portal_docker.json`; it is the MVP path.
- `conf/admin_portal.json` is retained as a future non-Docker starting point.
- Config files allow full-line `#` comments.
- URLs are direct base addresses; empty values hide the corresponding nav item.
- Prefer HAProxy frontends for gateway URLs so the portal never targets single instances.

## UI
- Serve the shared `ui/static/ui.css` for consistent styling.
- Theme toggle is owned by the portal; embedded UIs must not render their own toggle.
- All embedded consoles are HTML fragments only.

## HAProxy
- Render HAProxy status from the CSV stats endpoint (`/stats;csv`).
- Treat the CSV as the source of truth; do not scrape the HTML stats page.
