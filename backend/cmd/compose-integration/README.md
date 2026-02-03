# Compose integration runner

This command exercises end-to-end SubmissionManager behavior against the local Docker Compose stack.

Run from `backend/` after `docker compose up -d`:

    go run ./cmd/compose-integration

Optional flags:

- `-ui` runs Admin Portal send and troubleshoot checks (HTTP-level only; no browser automation). The default run does not include Admin Portal coverage.
- `-robust` runs only the longer exhaustion scenario and the restart recovery check (skips the default suite).

## Scenario coverage

The runner performs the following checks:

- Readiness: `/readyz` for SubmissionManager, Admin Portal, SMS gateway, Push gateway, and webhook-sink.
- SMS accepted: submit `sms.realtime`, expect terminal `accepted`, and verify webhook delivery.
- SMS rejected: submit `sms.realtime` with invalid recipient, expect `rejected` + `invalid_recipient`, verify webhook delivery.
- Exhausted: submit `sms.realtime` with message `FAIL`, wait for `exhausted` + `deadline_exceeded`, verify webhook delivery, and confirm multiple attempts in history.
- Idempotency: resubmitting the same intentId with same payload is accepted as idempotent; different payload returns HTTP 409.
- Restart recovery (optional): submit a failing intent, restart submission-manager, and verify attempts continue.
- Sync wait: submit with `waitSeconds` and expect a terminal response (not pending).
- Intent history: GET intent + history and confirm at least one attempt.
- Admin portal flows (optional): SMS and Push send endpoints return the expected intentId.
- Troubleshoot UI (optional): `/troubleshoot` renders and `/troubleshoot/history` returns intent details.
- Health: `/healthz` for SubmissionManager, Admin Portal, SMS gateway, Push gateway, and webhook-sink.

## Webhook sink note

The runner validates webhook delivery via the webhook-sink `/last` endpoint. If you just changed the webhook-sink code, restart that container so `/last` is available:

    docker compose restart webhook-sink
