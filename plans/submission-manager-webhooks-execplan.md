# SubmissionManager terminal webhooks

This ExecPlan is a living document. The sections Progress, Surprises & Discoveries, Decision Log, and Outcomes & Retrospective must be kept up to date as work proceeds.

This plan is written to satisfy `backend/PLANS.md` in the repository root and must be maintained in accordance with that file.

## Purpose / Big Picture

After this change, SubmissionManager will send a best-effort HTTP callback when an intent reaches a terminal status. Clients will be able to receive a single terminal notification without polling, while GET remains the source of truth. The callback will be configured on the submissionTarget contract and will be optional; if no webhook is configured, nothing changes.

## Progress

- [x] 2026-02-02 18:45Z Create initial ExecPlan for terminal webhooks.
- [x] 2026-02-02 19:10Z Add contract-level webhook config and validation in the registry.
- [x] 2026-02-02 19:18Z Extend SQL schema and store logic to persist webhook snapshots and delivery status.
- [x] 2026-02-02 19:28Z Add webhook dispatch to SubmissionManager with single-attempt best-effort delivery.
- [x] 2026-02-02 19:36Z Add tests for registry validation, persistence, and webhook dispatch paths.
- [x] 2026-02-02 19:45Z Update specs, README references, and CHANGELOG to reflect the new behavior.

## Surprises & Discoveries

- Observation: running all three packages in a single go test invocation can exceed the default tool timeout.
  Evidence: `go test ./submission ./submissionmanager ./cmd/submission-manager -count=1` timed out at 10s; reran with longer timeout.

## Decision Log

- Decision: Webhook delivery is best-effort with a single attempt and no retries in this phase.
  Rationale: Simpler behavior and aligns with explicit guidance to avoid retries at this stage.
  Date/Author: 2026-02-02 / Codex

- Decision: Webhook configuration is contract-level only and cannot be overridden by the intent request.
  Rationale: Prevents per-request arbitrary URLs and keeps behavior deterministic.
  Date/Author: 2026-02-02 / Codex

- Decision: Unsigned webhooks hard-fail unless a registry-level allowUnsignedWebhooks flag is enabled, and the config file must include a comment warning about this.
  Rationale: Enterprise deployments require signing; unsigned delivery is allowed only with explicit opt-in for non-production use.
  Date/Author: 2026-02-02 / Codex

## Outcomes & Retrospective

- Terminal webhooks are now contract-configurable with best-effort single-attempt delivery and unsigned gating via allowUnsignedWebhooks. The registry, SQL schema, store, and manager are updated, and tests cover validation and dispatch. Remaining gaps are intentional: no retries and no durable outbox yet.

## Context and Orientation

SubmissionTarget contracts are loaded and validated in `backend/submission/registry.go`, using a JSON file at `backend/conf/submission/submission_targets.json` (and the Docker variant). SubmissionManager core logic lives in `backend/submissionmanager/manager.go` and persists intents and attempts via `backend/submissionmanager/store.go` into SQL tables created by `backend/conf/sql/submissionmanager/001_create_schema.sql`. The HTTP server wiring for SubmissionManager lives under `backend/cmd/submission-manager`, and the existing gateway attempt executor is in `backend/cmd/submission-manager/executor.go`. Specs that define expected behavior live in `backend/spec/`, especially `backend/spec/submission-manager.md` and the new `backend/spec/submission-manager-webhooks.md`.

In this plan, “webhook” means a single HTTP POST sent when an intent becomes accepted, rejected, or exhausted. “Best-effort” means the system will try once and may miss a callback if the process crashes or the request fails.

## Plan of Work

First, update the webhook spec to remove retry language and to describe a single-attempt delivery model, as well as the allowUnsignedWebhooks requirement. Next, extend the registry schema and validation in `backend/submission/registry.go` to accept a `webhook` object on each target and a registry-level `allowUnsignedWebhooks` flag. The validation must enforce that if a webhook is configured and no secretEnv is provided, the registry only loads when allowUnsignedWebhooks is true. Update the sample registry JSON files to include an explicit allowUnsignedWebhooks field and a full-line comment warning that unsigned webhooks are not allowed in enterprise deployments.

Then, extend the SQL schema to store the webhook snapshot and delivery status on the intent row. Add columns for webhook URL, headers JSON, headersEnv JSON, secretEnv, webhook_status, webhook_attempted_at, and webhook_error. Update insert and load paths in `backend/submissionmanager/store.go` to persist and hydrate this data into the contract snapshot. Add a store method to mark a single webhook attempt as delivered or failed with a timestamp and error message. There is no separate scheduling table in this phase.

Next, update `backend/submissionmanager/manager.go` to trigger a webhook send after a terminal attempt is recorded. This should be a single attempt. If a webhook is not configured or the delivery status is already set, it should do nothing. Create a new function type in submissionmanager, similar to AttemptExecutor, for webhook delivery. Wire an HTTP implementation in `backend/cmd/submission-manager` that resolves headersEnv and secretEnv from environment variables, constructs the JSON payload, signs it when a secret is present, and performs the POST. The HTTP sender should return an error for non-2xx responses so the manager can mark the attempt as failed.

Finally, add tests. Extend registry tests in `backend/submission/registry_test.go` to cover webhook config validation, including the unsigned gating. Add store tests in `backend/submissionmanager/manager_test.go` or a new store test file to verify that webhook fields are persisted and loaded. Add manager tests that confirm a webhook is attempted exactly once on terminal intents and is not attempted without configuration. Update specs and README files (`backend/spec/README.md`, `backend/spec/submission-manager.md`, `backend/spec/submission-manager-webhooks.md`) plus `CHANGELOG.md` to reflect the new behavior.

## Concrete Steps

Work in the repository root.

1) Update specs to remove retry language and add unsigned gating. Edit:

   - backend/spec/submission-manager-webhooks.md
   - backend/spec/submission-manager.md

2) Update registry configuration and validation:

   - backend/submission/registry.go
   - backend/submission/registry_test.go
   - backend/conf/submission/submission_targets.json
   - backend/conf/submission/submission_targets_docker.json

3) Update SQL schema and store logic:

   - backend/conf/sql/submissionmanager/001_create_schema.sql
   - backend/submissionmanager/store.go

4) Implement webhook dispatch and sender wiring:

   - backend/submissionmanager/manager.go
   - backend/cmd/submission-manager/executor.go (new webhook sender helper)
   - backend/cmd/submission-manager/main.go (wire sender into manager)

5) Add tests and run:

   - go test ./submission ./submissionmanager ./cmd/submission-manager -count=1

6) Update docs and changelog:

   - backend/spec/README.md
   - backend/submissionmanager/README.md
   - CHANGELOG.md

## Validation and Acceptance

Run tests:

   (from backend/) go test ./submission ./submissionmanager ./cmd/submission-manager -count=1

Manual check (optional, but recommended) using a local webhook receiver:

   - Start a simple HTTP receiver that prints requests, for example with:
       (from a separate shell) python - <<'PY'
       from http.server import BaseHTTPRequestHandler, HTTPServer
       class H(BaseHTTPRequestHandler):
           def do_POST(self):
               length = int(self.headers.get('content-length', '0'))
               body = self.rfile.read(length)
               print("headers:", dict(self.headers))
               print("body:", body.decode())
               self.send_response(200)
               self.end_headers()
       HTTPServer(("0.0.0.0", 9999), H).serve_forever()
       PY

   - Configure a submissionTarget webhook URL to http://localhost:9999 and submit an intent that reaches terminal. Confirm the POST is printed and the intent is still visible via GET /v1/intents/{id}.

Acceptance criteria:

   - Registry rejects webhook config without secretEnv when allowUnsignedWebhooks is false.
   - Terminal intents trigger at most one webhook attempt, and no webhook attempt happens for non-terminal intents.
   - Webhook payload matches the spec and includes intent terminal fields.

## Idempotence and Recovery

All edits are additive and safe to re-apply. SQL schema changes are written with IF guards to allow re-running the migration. If a webhook attempt fails, the intent remains terminal and can be fetched via GET; webhooks are best-effort and may be missed by design in this phase.

## Artifacts and Notes

Keep diffs small and focused. If you change the registry JSON, include a comment line explaining the allowUnsignedWebhooks flag. If you update SQL schema, include a brief note in the change log to make the new columns discoverable.

## Interfaces and Dependencies

In `backend/submission/registry.go`, define a WebhookConfig struct on TargetContract with fields:

   - URL string
   - Headers map[string]string
   - HeadersEnv map[string]string
   - SecretEnv string

Add a registry-level allowUnsignedWebhooks boolean that is enforced during validation.

In `backend/submissionmanager/manager.go`, define a function type:

   WebhookSender func(context.Context, WebhookDelivery) error

where WebhookDelivery includes the resolved URL, headers, signature, and JSON payload. The core submissionmanager package must not import net/http. The concrete HTTP sender belongs in `backend/cmd/submission-manager`.

At the end of each iteration, append a short note to this ExecPlan describing what changed and why.

---
Plan created: 2026-02-02 / Codex. Future edits must update Progress, Decision Log, and Outcomes.

Update note 2026-02-02: marked registry, schema/store, dispatch, and tests as complete; recorded the test timeout observation.
Update note 2026-02-02: marked spec/README/changelog updates as complete.
Update note 2026-02-02: recorded final outcomes and left retries/outbox as intentional gaps.
