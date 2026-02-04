# Intent history UI (authoritative attempts view)

This execplan is a living document. The sections Progress, Surprises & Discoveries, Decision Log, and Outcomes & Retrospective must be kept up to date as work proceeds.

This plan follows backend/PLANS.md from the repository root. Keep it updated as changes are made.

## Purpose / Big Picture

Operators should be able to see the authoritative intent status and attempt history without relying on logs. After this change, the admin portal Troubleshoot page includes a new “Intent history” panel that shows the intent summary and its attempts. This panel is backed by SubmissionManager and is the primary debugging surface; log views are removed from the front-end. You can see it working by opening the SMS or Push troubleshoot page and submitting an intentId; the history panel should show the intent state and the attempts table.

## Progress

- [x] (2026-01-30 18:45Z) Inspect SubmissionManager storage APIs and existing UI proxy patterns to confirm the simplest history surface.
- [x] (2026-01-30 19:10Z) Add a JSON history endpoint in SubmissionManager and a matching HTML fragment endpoint for the portal to proxy.
- [x] (2026-01-30 19:25Z) Add an “Intent history” panel to the portal troubleshoot page and proxy handlers.
- [x] (2026-01-30 19:35Z) Update specs/README to describe the new history view and its semantics.
- [x] (2026-01-30 19:45Z) Add tests and run them.
- [ ] (2026-01-30) Validate in docker compose and capture expected behavior.

## Surprises & Discoveries

- None yet.

## Decision Log

- Decision: Keep the admin portal as a thin proxy by adding a SubmissionManager HTML fragment endpoint for history, rather than parsing JSON in the portal.
  Rationale: Preserves the “proxy only” boundary in admin-portal AGENTS and keeps logic in SubmissionManager.
  Date/Author: 2026-01-30 / Codex

- Decision: History JSON endpoint returns a wrapper object `{ intent, attempts }` with `attempts` always present (possibly empty).
  Rationale: Keeps the response explicit and avoids null ambiguity in clients.
  Date/Author: 2026-01-30 / Codex

- Decision: Remove gateway/manager log panels from the portal troubleshoot view.
  Rationale: Logs are instance-local under load balancing, so the portal would frequently show empty results; the authoritative history panel is now the primary surface.
  Date/Author: 2026-01-30 / Codex

- Decision: Remove gateway and manager troubleshoot log UIs from the front-end entirely.
  Rationale: Front-end log views are misleading under load balancing; logs remain server-side only.
  Date/Author: 2026-01-30 / Codex

## Outcomes & Retrospective

- Pending.

## Context and Orientation

SubmissionManager persists intents and attempts in SQL Server and already exposes JSON APIs at `/v1/intents` and `/v1/intents/{id}`. The storage layer can load attempts (`loadAttempts`) and `loadIntent` already attaches attempts to an Intent in `backend/submissionmanager/store.go`.

The admin portal renders a Troubleshoot page at `/sms/ui/troubleshoot` and `/push/ui/troubleshoot` with an intent history panel. It proxies SubmissionManager HTML fragments rather than parsing or interpreting upstream responses.

Definitions used in this plan:

- “Intent history”: the authoritative view of an intent’s status plus its attempts, sourced from SQL persistence.
- “Attempt history”: the ordered list of attempts for an intent (attempt number, start/end time, outcome, error).
- “History panel”: a new section on the portal Troubleshoot page that renders the intent summary and attempts list.

## Plan of Work

1) SubmissionManager history surface.

Add a new JSON endpoint and a lightweight HTML fragment endpoint.

- JSON endpoint: `GET /v1/intents/{intentId}/history` returns the intent summary and attempts list. This should use the existing manager/store data, without new persistence logic.

- HTML fragment endpoint: `POST /ui/history` accepts form `intentId` and returns an HTML fragment suitable for HTMX embedding. This will be used by the admin portal to render the history panel without JSON parsing in the portal.

Add two templates under `ui/` for the history panel fragment (title + summary + attempts table). Keep the copy explicit that it is authoritative and sourced from persistence.

2) Admin portal history panel.

Extend `ui/portal_troubleshoot.tmpl` to include a new panel labeled “Intent history”. The panel contains a form with `intentId` and posts to a new portal handler that proxies to SubmissionManager `/ui/history`.

Add new portal handlers for:

- `POST /sms/ui/troubleshoot/history` -> proxy to SubmissionManager `/ui/history`
- `POST /push/ui/troubleshoot/history` -> proxy to SubmissionManager `/ui/history`

Ensure these handlers validate intentId and SubmissionManager URL and use `renderError` for UI errors per `backend/cmd/admin-portal/AGENTS.md`.

3) Specs and docs.

Update:

- `specs/submission-manager.md` to document `/v1/intents/{id}/history` (JSON) and `/ui/history` (HTML fragment), plus ordering and semantics (attempts ordered ascending; authoritative; no payload/contract snapshot).
- `specs/admin-portal.md` to describe the new “Intent history” panel and endpoints.
- If necessary, `backend/cmd/admin-portal/README.md` to mention the history panel in the Troubleshoot flow.

4) Tests.

Add tests:

- SubmissionManager HTTP handler test for `GET /v1/intents/{id}/history` returning intent summary and attempts.
- SubmissionManager UI handler test for `POST /ui/history` returning the fragment when intentId is present.
- Admin portal tests for `/sms/ui/troubleshoot/history` proxying to submission-manager and returning the HTML fragment.

## Concrete Steps

1. SubmissionManager: history JSON endpoint.

- Add a new handler in `backend/cmd/submission-manager/handlers.go`:

  - `handleHistory` validates method and intentId (from path), loads intent via `manager.GetIntent`, and writes JSON using a new response struct that includes attempts.

- Register the route in `backend/cmd/submission-manager/routes.go` as `/v1/intents/` with a subpath check, or add a separate mux handler for `/v1/intents/` to detect `/history` suffix.

2. SubmissionManager: history HTML fragment endpoint.

- Extend `backend/cmd/submission-manager/ui.go` to load new templates (e.g., `manager_history.tmpl` and `manager_history_results.tmpl`).
- Add `handleHistory` for `POST /ui/history` that accepts form `intentId`, calls `manager.GetIntent`, and renders the fragment using the new templates.

3. Portal: history panel + proxy.

- Update `ui/portal_troubleshoot.tmpl` to include the new panel with its own `hx-target` container.
- Add handlers in `backend/cmd/admin-portal/troubleshoot.go` for `/sms/ui/troubleshoot/history` and `/push/ui/troubleshoot/history` that proxy to SubmissionManager `/ui/history`.
- Register routes in `backend/cmd/admin-portal/main.go`.

4. Tests + validation.

- Add tests in `backend/cmd/submission-manager/main_test.go` for history JSON and fragment endpoints.
- Add tests in `backend/cmd/admin-portal/main_test.go` for the portal history proxy.
- Run tests:

  (from `backend/`)
  go test ./cmd/submission-manager -count=1
  go test ./cmd/admin-portal -count=1

## Validation and Acceptance

Run the tests listed above and then validate in docker compose:

- `docker compose up -d`
- Open `http://localhost:8090/sms/ui` → Troubleshoot.
- Enter an intentId in the “Intent history” panel and submit.
- Confirm the panel shows intent summary and attempts list. Log panels remain unchanged and optional.

Acceptance criteria:

- Intent history is visible in the portal and sourced from SubmissionManager persistence.
- The history panel updates independently of logs.
- Logs remain server-side only and are not exposed in the UI.

## Idempotence and Recovery

Changes are additive and safe to re-run. If the history endpoints are undesirable, remove the portal routes and UI panel; the logs remain unaffected. If template loading fails, the portal still serves other panels; errors are visible via `renderError`.

## Artifacts and Notes

Example history panel fragment (simplified):

  <section class="panel">
    <h2>Intent history</h2>
    <dl class="kv">
      <dt>Status</dt><dd>accepted</dd>
      <dt>Created</dt><dd>2026-01-30T10:00:00Z</dd>
    </dl>
    <table class="table">
      <thead><tr><th>Attempt</th><th>Started</th><th>Finished</th><th>Outcome</th><th>Error</th></tr></thead>
      <tbody>...</tbody>
    </table>
  </section>

Test output (abbreviated):

  $ (cd backend && go test ./cmd/submission-manager -count=1)
  ok  	gateway/cmd/submission-manager	2.631s

  $ (cd backend && go test ./cmd/admin-portal -count=1)
  ok  	gateway/cmd/admin-portal	0.566s

Plan update note: Marked implementation and test steps complete, recorded the history response shape decision, removed portal and gateway log UIs due to load-balancing limitations, and added test transcripts.

## Interfaces and Dependencies

- SubmissionManager:
  - GET `/v1/intents/{intentId}/history` → JSON intent + attempts.
  - POST `/ui/history` → HTML fragment for history.

- Admin portal:
  - POST `/sms/ui/troubleshoot/history` and `/push/ui/troubleshoot/history` → proxy to SubmissionManager `/ui/history`.

Dependencies:

- Go standard library only.
- UI templates in repo root `ui/`.
- Admin portal remains a thin proxy; no JSON parsing or business logic in portal.
