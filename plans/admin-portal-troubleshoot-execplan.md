# Admin portal combined troubleshoot view (manager + gateway)

This ExecPlan is a living document. The sections Progress, Surprises & Discoveries, Decision Log, and Outcomes & Retrospective must be kept up to date as work proceeds.

This plan follows backend/PLANS.md from the repository root. Keep it updated as changes are made.

## Purpose / Big Picture

Operators can click the existing Troubleshoot link inside the gateway console and see two separate log panels: one for SubmissionManager activity and one for the gateway itself. Each panel is independently queried by intentId and displays only its own service output. This makes troubleshooting deterministic without merging logs or adding any new execution logic.

You can see it working by opening the SMS or Push console in the admin portal, clicking Troubleshoot, and then fetching logs for an intentId. You should see a “SubmissionManager logs” section and a “Gateway logs” section, each populated via its own request.

## Progress

- [x] (2026-01-30 16:35Z) Read current admin portal + submission-manager UI routing and log buffer patterns; confirm templates and routes to extend.
- [x] (2026-01-30 16:55Z) Implement SubmissionManager log buffer + troubleshoot UI endpoint with intentId filtering.
- [x] (2026-01-30 17:15Z) Implement admin portal combined troubleshoot page and proxy handlers for manager/gateway log panels.
- [x] (2026-01-30 17:25Z) Update specs and portal docs to reflect new troubleshoot flow and in-memory log scope.
- [x] (2026-01-30 18:05Z) Add tests for new handlers and troubleshoot flow; ran cmd/admin-portal and cmd/submission-manager tests.
- [ ] (2026-01-30) Validate end-to-end with docker compose and document expected behavior.

## Surprises & Discoveries

- None yet.

## Decision Log

- Decision: Intercept /sms/ui/troubleshoot and /push/ui/troubleshoot in the admin portal to render a combined view rather than rewriting gateway HTML.
  Rationale: Preserves the existing troubleshoot link without merging streams, keeps routing explicit, and avoids brittle HTML rewrites.
  Date/Author: 2026-01-30 / Codex

- Decision: Provide a SubmissionManager troubleshoot UI endpoint that returns HTML fragments for HTMX requests.
  Rationale: Keeps the admin portal a thin proxy, avoids new JSON adapters, and matches existing gateway UI patterns.
  Date/Author: 2026-01-30 / Codex

- Decision: The SubmissionManager troubleshoot UI always renders HTML fragments (no standalone HTML shell).
  Rationale: The admin portal is the primary consumer, and fragments keep the implementation minimal without extra static asset serving.
  Date/Author: 2026-01-30 / Codex

## Outcomes & Retrospective

Completed the combined troubleshoot page with log panels, then later removed all front-end log panels due to load-balancing limitations. The authoritative intent history panel now lives in `plans/intent-history-execplan.md`.

## Context and Orientation

The admin portal (`backend/cmd/admin-portal`) is a thin UI proxy that renders a Setu shell and forwards UI/API requests to SMS gateway, push gateway, HAProxy, and services-health. It already rewrites gateway UI paths, routes `/sms/send` and `/push/send` through SubmissionManager when configured, and renders submission responses in `ui/submission_result.tmpl`.

Gateways expose a Troubleshoot UI at `/ui/troubleshoot` that uses an in-memory log buffer to show log lines filtered by referenceId. The admin portal currently proxies this endpoint as `/sms/ui/troubleshoot` or `/push/ui/troubleshoot` via `proxyUI`.

SubmissionManager (`backend/cmd/submission-manager`) currently exposes only JSON endpoints at `/v1/intents` and `/v1/intents/{id}`. It has no UI and no log buffer.

The shared UI templates live in the repo root `ui/` directory. Admin portal templates are loaded from there via `loadPortalTemplates`. Gateways also read from `ui/` and render HTMX fragments when `HX-Request: true` is set.

Definitions used in this plan:

- “Troubleshoot page”: a portal page that shows two independent log panels, one for SubmissionManager, one for the gateway.
- “Log buffer”: a bounded in-memory ring buffer of recent log lines. It is best-effort and non-durable; it resets on restart.
- “HTMX request”: a request with header `HX-Request: true` that expects an HTML fragment instead of a full HTML page.

## Plan of Work

Add a minimal SubmissionManager troubleshoot UI that can be queried by intentId and returns HTML fragments suitable for embedding in the admin portal. The log source will be an in-memory log buffer that captures SubmissionManager log lines. Ensure log lines include `intentId=...` so the buffer can filter correctly.

Then add an admin portal troubleshoot page that is served from `/sms/ui/troubleshoot` and `/push/ui/troubleshoot`. The page will render two sections with independent forms: “SubmissionManager logs” and “Gateway logs”. Each form posts to a separate portal endpoint and updates only its own result panel. The portal will proxy the form submission to the appropriate upstream (SubmissionManager or gateway) and return the upstream HTML fragment. The gateway query uses the same intentId value, sent as `referenceId` in the gateway form payload.

Update admin portal routes so the combined troubleshoot page overrides the proxied gateway troubleshoot route. This must not merge log streams or interpret upstream responses. The portal should only proxy and display fragments.

Update `backend/spec/admin-portal.md` to describe the new troubleshoot entry points and the two-panel flow. Update `backend/spec/submission-manager.md` to document the troubleshoot endpoint and the fact that logs are in-memory only.

Add tests to verify the new handlers and the proxy behavior. Keep tests boring and behavior-driven.

## Concrete Steps

1. Add SubmissionManager troubleshoot support.

   - In `backend/cmd/submission-manager`, introduce a `logBuffer` type modeled after the gateway log buffer. Provide:

     - A `Write` method that splits log lines.
     - `entriesForIntentID(intentID string, limit int)` that filters lines containing `intentId="<id>"`.

   - In `backend/cmd/submission-manager/main.go`, create the log buffer and set `log.SetOutput(io.MultiWriter(os.Stderr, logBuffer))` early in main, before constructing the manager.

   - In `backend/submissionmanager/manager.go`, add minimal log statements that include `intentId=%q` so the buffer has something to show. Add log lines when:

     - An attempt begins (include attempt number and gateway type).
     - An attempt finishes (include outcome status/reason or error, and whether a retry is scheduled).
     - An intent is exhausted or accepted/rejected (include final status).

     Keep logging simple and avoid large refactors.

   - Add a small UI server in `backend/cmd/submission-manager` (new file such as `ui.go`) that exposes:

     - `GET /ui/troubleshoot`: renders a simple page with an intentId form (optional but helpful for direct access).
     - `POST /ui/troubleshoot`: validates `intentId`, fetches log entries from the log buffer, and returns an HTML fragment.

     Use new templates under `ui/` for SubmissionManager-specific troubleshooting (for example: `manager_troubleshoot.tmpl` and `manager_troubleshoot_results.tmpl`). Keep copy intentId-specific and remove gateway-only language.

   - Update `backend/cmd/submission-manager/routes.go` to register the new UI endpoints alongside `/v1/intents` routes.

2. Add admin portal combined troubleshoot page.

   - Create a new template in `ui/portal_troubleshoot.tmpl` that contains two panels:

     - SubmissionManager logs panel with a form that posts to `/sms/ui/troubleshoot/manager` or `/push/ui/troubleshoot/manager` depending on which console is active.
     - Gateway logs panel with a form that posts to `/sms/ui/troubleshoot/gateway` or `/push/ui/troubleshoot/gateway`.

     Each panel has its own `hx-post` and `hx-target` with separate result containers to avoid merged streams. Label the input as `intentId`.

   - Update `backend/cmd/admin-portal/templates.go` and `backend/cmd/admin-portal/types.go` to load this template and pass a view struct with the correct action URLs and headings.

   - Add handlers in `backend/cmd/admin-portal/handlers.go` (or a new file if that keeps code clearer) for:

     - `GET /sms/ui/troubleshoot` and `GET /push/ui/troubleshoot`: render the combined troubleshoot page.
     - `POST /sms/ui/troubleshoot/manager` and `/push/ui/troubleshoot/manager`: proxy the form data to `SubmissionManagerURL + /ui/troubleshoot`.
     - `POST /sms/ui/troubleshoot/gateway` and `/push/ui/troubleshoot/gateway`: proxy the form data to the gateway `/ui/troubleshoot` endpoint.

     Each POST handler must validate that its upstream URL is configured and that intentId is present. For gateway calls, translate form field `intentId` to `referenceId` in the outbound request body.

   - Use the existing `renderError` for UI error responses, per `backend/cmd/admin-portal/AGENTS.md`.

   - Register these routes in `backend/cmd/admin-portal/main.go`. Ensure the `/sms/ui/troubleshoot` and `/push/ui/troubleshoot` routes are declared so they take precedence over the `/sms/ui/` and `/push/ui/` proxy handlers (ServeMux matches the longest pattern).

3. Update specs and docs.

   - In `backend/spec/admin-portal.md`, add the new troubleshoot endpoints and describe the two-panel log view, with the note that it proxies upstream logs and does not merge them.

   - In `backend/spec/submission-manager.md`, add a short section describing `/ui/troubleshoot`, the intentId filter, and that the log buffer is in-memory and best-effort.

   - If any README mentions troubleshooting routes, update accordingly.

4. Add tests.

   - In `backend/cmd/admin-portal/main_test.go`, add tests that:

     - `GET /sms/ui/troubleshoot` returns the portal troubleshoot HTML with both sections.
     - `POST /sms/ui/troubleshoot/manager` proxies to the submission manager and returns its fragment.
     - `POST /sms/ui/troubleshoot/gateway` proxies to the gateway and returns its fragment (validate that intentId is forwarded as referenceId).

   - In `backend/cmd/submission-manager/main_test.go`, add tests that:

     - `POST /ui/troubleshoot` returns a fragment when intentId is supplied and the log buffer has a matching line.
     - Missing intentId yields a 400 response.

5. Run tests and validate end-to-end.

## Validation and Acceptance

Run tests from `backend/`:

  go test ./cmd/admin-portal -count=1
  go test ./cmd/submission-manager -count=1

Start the stack (from repo root):

  docker compose up -d

Then validate in a browser:

- Open `http://localhost:8090/sms/ui`, click Troubleshoot. You should see two panels: “SubmissionManager logs” and “Gateway logs.”
- Enter an intentId that exists in recent SMS submissions. The manager panel should show log lines containing that intentId. The gateway panel should show gateway log entries for the same intentId.
- Repeat in `http://localhost:8090/push/ui` and confirm the push gateway panel is used.

Acceptance criteria:

- The Troubleshoot link routes to the portal’s combined page, not the gateway-only page.
- The manager and gateway panels update independently and never merge outputs.
- No business logic is added to the admin portal; it only validates input and proxies upstream responses.

## Idempotence and Recovery

All changes are additive and can be re-run safely. If template loading fails, revert by removing the new template entries and routes. If the new troubleshoot endpoints are undesirable, comment out the `/sms/ui/troubleshoot*` and `/push/ui/troubleshoot*` routes to fall back to the gateway’s native troubleshoot UI.

## Artifacts and Notes

Expected troubleshooting form structure (HTML fragment example):

  <section class="panel">
    <h2>SubmissionManager logs</h2>
    <form method="post" action="/sms/ui/troubleshoot/manager" hx-post="/sms/ui/troubleshoot/manager" hx-target="#manager-log-results" hx-swap="innerHTML">
      <label for="intentId">intentId</label>
      <input id="intentId" name="intentId" type="text" required />
    </form>
  </section>

Test output (abbreviated):

  $ (cd backend && go test ./cmd/admin-portal -count=1)
  ok  	gateway/cmd/admin-portal	0.505s

  $ (cd backend && go test ./cmd/submission-manager -count=1)
  ok  	gateway/cmd/submission-manager	2.336s

## Interfaces and Dependencies

SubmissionManager (new UI endpoints):

- GET `/ui/troubleshoot` -> HTML page for manual use (optional).
- POST `/ui/troubleshoot` -> HTML fragment showing log lines filtered by intentId.

Admin portal (new routes):

- GET `/sms/ui/troubleshoot` and `/push/ui/troubleshoot` -> combined troubleshoot page.
- POST `/sms/ui/troubleshoot/manager` and `/push/ui/troubleshoot/manager` -> proxy to SubmissionManager `/ui/troubleshoot`.
- POST `/sms/ui/troubleshoot/gateway` and `/push/ui/troubleshoot/gateway` -> proxy to gateway `/ui/troubleshoot`.

Dependencies:

- Use only Go standard library. No new third-party dependencies.
- Reuse the shared `ui/` templates and CSS.
- Treat SubmissionManager and gateways as upstream services; the admin portal must not interpret or merge their log output.

Plan update note: Marked core implementation and test steps complete, documented the fragment-only UI decision, aligned the missing intentId expectation with the 400 response, and added abbreviated test transcripts.
