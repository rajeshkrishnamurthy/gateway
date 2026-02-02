## Codex maintenance notes

This repo is maintained by Codex across sessions. Before changing behavior, read the closest `AGENTS.md` and follow `PLANS.md` for non-trivial work.
ExecPlans live under `plans/`.

Start with these operational docs:
- `backend/cmd/services-health/README.md` for the Command Center.
- `backend/cmd/admin-portal/README.md` for the Admin Portal.

Prefer explicit, minimal changes that preserve the current submission-only contracts.

## SubmissionManager (Phase 1)

Phase 1 defines the SubmissionTarget registry and contract validation only (no execution engine yet).

- `backend/spec/submission-manager.md` defines SubmissionIntent, gatewayType, submissionTarget, and contract semantics.
- `backend/conf/submission/submission_targets.json` is the sample registry for SubmissionManager.
- `backend/submission/README.md` describes the registry loader behavior.

## SubmissionManager (Phase 2/3a)

Phase 2 introduced the SubmissionManager execution engine. Phase 3a adds SQL Server durability for intents, attempts, and scheduling metadata.

- `backend/submissionmanager/README.md` describes the execution engine and its boundaries.
- `backend/conf/sql/submissionmanager/001_create_schema.sql` defines the persistence schema.

## SubmissionManager HTTP (Phase 3b)

Phase 3b exposes SubmissionManager over HTTP as a thin adapter.

Run from `backend/`:

```sh
go run ./cmd/submission-manager \\
  -addr :8082 \\
  -registry conf/submission/submission_targets.json \\
  -sql-host localhost \\
  -sql-port 1433 \\
  -sql-user sa \\
  -sql-password \"$MSSQL_SA_PASSWORD\" \\
  -sql-db setu \\
  -sql-encrypt disable
```

Endpoints:

- POST `http://localhost:8082/v1/intents` (optional `waitSeconds` query param for synchronous wait)
- GET `http://localhost:8082/v1/intents/{intentId}`

Response JSON:

- intentId
- submissionTarget
- status (pending, accepted, rejected, exhausted)
- createdAt (RFC3339)
- completedAt (present when terminal)
- rejectedReason (present when status is rejected)
- exhaustedReason (present when status is exhausted)

Docker Compose (dev/testing):

```sh
docker compose up -d
```

The compose service uses `conf/submission/submission_targets_docker.json` so the manager reaches HAProxy by service name.

Smoke test (HTTP):

```sh
curl -sS -X POST http://localhost:8082/v1/intents \
  -H 'Content-Type: application/json' \
  -d '{
    "intentId": "intent-1",
    "submissionTarget": "sms.realtime",
    "payload": {
      "referenceId": "intent-1",
      "to": "+15551234567",
      "message": "hello"
    }
  }'

curl -sS http://localhost:8082/v1/intents/intent-1
```

## SQL Server (local dev, Phase 3a)

For macOS development, SQL Server runs in Docker via `docker-compose.yml` as the `mssql` service.
Credentials are stored in `backend/.env` (git-ignored).

Connection details:

- Host: `localhost`
- Port: `1433`
- User: `sa`
- Password: from `backend/.env` (`MSSQL_SA_PASSWORD`)

Schema updates:

- If your database predates the removal of `submission_intents.mode`, re-run `backend/conf/sql/submissionmanager/001_create_schema.sql` to drop the column.

## MVP posture (Docker-only)

For the MVP, Setu is operated via Docker Compose only. Non-Docker run paths are intentionally out of scope to keep dev/devops overhead low.

Configs and infra files under `conf/` are kept to leave the door open for a future non-Docker deployment, but they are not maintained for direct host execution today.

## SMS REST contract

Gateway request (JSON):

```json
{
  "referenceId": "string",
  "to": "string",
  "message": "string",
  "tenantId": "string (optional)"
}
````

Gateway response (JSON):

```json
{
  "referenceId": "string",
  "status": "accepted|rejected",
  "gatewayMessageId": "string (present when accepted)",
  "reason": "invalid_request|duplicate_reference|invalid_recipient|invalid_message|provider_failure (present when rejected)"
}
```


## Push REST contract

Gateway request (JSON):

```json
{
  "referenceId": "string",
  "token": "string",
  "title": "string (optional)",
  "body": "string (optional)",
  "data": {"key": "value"} (optional),
  "tenantId": "string (optional)"
}
```

At least one of `title`, `body`, or `data` must be present.

Gateway response (JSON):

```json
{
  "referenceId": "string",
  "status": "accepted|rejected",
  "gatewayMessageId": "string (present when accepted)",
  "reason": "invalid_request|duplicate_reference|provider_failure|unregistered_token (present when rejected)"
}
```

## HTTP status semantics

Gateways must never return 2xx unless they can produce a complete, valid normalized outcome.

200 + status=rejected
Submission was attempted but not confirmed.
The client may retry later with the same or a new referenceId, based on its own policy.

200 + status=accepted
The request was successfully submitted to the provider.

non-2xx
Returned only when the gateway cannot produce a complete, valid normalized outcome.

## Submission vs delivery

An accepted response means the request was successfully submitted to the provider.

It does not guarantee:

* message delivery
* eventual delivery status
* retries on failure
* reconciliation

Delivery outcomes, if required, must be handled by systems outside the gateway.

## gatewayMessageId semantics

gatewayMessageId is generated by the gateway and is present only when status=accepted.
It represents confirmed submission, not delivery.
It must not be assumed to exist:
* when submission outcome is ambiguous
* when the provider fails or times out

## Idempotency

Gateway enforces idempotency only for concurrent in-flight requests within a single process using referenceId.
It does not guarantee idempotency across time, retries, or restarts, so a duplicate after a request completes may be accepted.

## Metrics

Gateway exposes Prometheus metrics at `/metrics`.
Metrics are low-cardinality and use the adapter provider name for the `provider` label.
Latency histograms share buckets at 0.1s, 0.25s, 0.5s, 1s, 2.5s, and 5s.


## Gateway stack overview

The gateway stack is intentionally small and explicit:

- Gateways: `cmd/sms-gateway` and `cmd/push-gateway` run as containers. Provider semantics and instance-agnostic settings (like `grafanaDashboardUrl`) live in `conf/docker/config_docker.json` and `conf/docker/config_push_docker.json`. Secrets are provided via environment variables only.
- HAProxy: Docker Compose uses `conf/docker/haproxy_docker.cfg` to front stable ports and route to multiple gateway instances. Gateways remain unaware of peers.
- Prometheus: Docker Compose uses `conf/docker/prometheus_docker.yml` to scrape gateway instances directly (do not scrape HAProxy). Jobs separate SMS vs push.
- Grafana: provisioned dashboards live under `conf/grafana/dashboards`. The gateway UI Metrics link points to `grafanaDashboardUrl` from the gateway config (defaults to the SMS/push dashboard URLs).

## Services health console

The services health console is a host-run UI for checking container status (up/down) and running Docker Compose start/stop commands defined in `conf/docker/services_health.json`.

Start the console from `backend/`:

```sh
go run ./cmd/services-health -config conf/docker/services_health.json -addr :8070
```

Open `http://localhost:8070/ui`.

Notes:
- Status checks use HTTP GET to each instance `healthUrl` (2xx = up).
- Each instance in `conf/docker/services_health.json` must define `healthUrl`.
- Start/stop actions execute the command arrays from the config with `{config}`, `{addr}`, and `{port}` placeholder substitution.
- Relative paths are resolved from the health console working directory.
- Docker Compose must be available on the host PATH.

## Docker Compose quick start

For a cross-platform dev stack, use Docker Compose from the repo root:

```sh
docker compose up
```

Then open:

- Admin portal: `http://localhost:8090/ui`
- Grafana: `http://localhost:3000` (default credentials `admin` / `admin`)
- Prometheus: `http://localhost:9090`

Compose uses:

- `conf/docker/config_docker.json` and `conf/docker/config_push_docker.json` for gateways.
- `conf/docker/admin_portal_docker.json` for the admin portal (Command Center hosted on `host.docker.internal:8070`).
- `conf/docker/prometheus_docker.yml` and `conf/docker/haproxy_docker.cfg` for infra.

If you want the Command Center inside the admin portal while running Docker Compose, start it on the host from `backend/`:

```sh
go run ./cmd/services-health -config conf/docker/services_health.json -addr :8070
```

The Docker admin portal config points at `http://host.docker.internal:8070` for the Command Center URL.

The gateway containers run `go run` using the `golang:tip` image to match the `go 1.25` module declaration. If you want a pinned Go version, update the image tags and ensure they support the `go.mod` version.

## Scaling gateways (Docker Compose)

To run more instances, add services in `docker-compose.yml` and update `conf/docker/haproxy_docker.cfg` and `conf/docker/prometheus_docker.yml` to include the new backends and scrape targets. Keep host ports unique when exposing additional instances.

## Gateway smoke test (Docker)

Send requests through the SMS gateway (use a fresh referenceId each time):

```sh
curl -i -X POST http://localhost:8080/sms/send \
  -H 'Content-Type: application/json' \
  -d '{"referenceId":"ref-1","to":"15551234567","message":"hello"}'

curl -i -X POST http://localhost:8080/sms/send \
  -H 'Content-Type: application/json' \
  -d '{"referenceId":"ref-2","to":"abc","message":"hello"}'

curl -i -X POST http://localhost:8080/sms/send \
  -H 'Content-Type: application/json' \
  -d '{"referenceId":"ref-3","to":"15551234567","message":"                     "}'

curl -i -X POST http://localhost:8080/sms/send \
  -H 'Content-Type: application/json' \
  -d '{"referenceId":"ref-4FAIL","to":"15551234567","message":"hello"}'
```

## Model provider (adapter demo)

Docker Compose runs the model provider container on `http://model-provider:9091`. The SMS docker config already points to it:

```json
{
  "smsProvider": "model",
  "smsProviderUrl": "http://model-provider:9091/sms/send",
  "smsProviderConnectTimeoutSeconds": 2,
  "smsProviderTimeoutSeconds": 30,
  "grafanaDashboardUrl": "http://localhost:3000/d/gateway-overview-sms"
}
```

Note: the model provider intentionally adds a random 50ms-2s delay for manual latency testing (TODO: remove).

## sms24x7 provider

Set the API key via `SMS24X7_API_KEY` in the environment (do not put secrets in config files). Update `conf/docker/config_docker.json` to use this provider.

```json
{
  "smsProvider": "sms24x7",
  "smsProviderUrl": "https://api.example.com/sms/send",
  "smsProviderServiceName": "your-service",
  "smsProviderSenderId": "your-sender",
  "smsProviderConnectTimeoutSeconds": 2,
  "smsProviderTimeoutSeconds": 30
}
```

## smskarix provider

Set the API key via `SMSKARIX_API_KEY` in the environment (do not put secrets in config files). Update `conf/docker/config_docker.json` to use this provider.

```json
{
  "smsProvider": "smskarix",
  "smsProviderUrl": "https://api.example.com/sms/send",
  "smsProviderVersion": "v1",
  "smsProviderSenderId": "your-sender",
  "smsProviderConnectTimeoutSeconds": 2,
  "smsProviderTimeoutSeconds": 30
}
```

## smsinfobip provider

Set the API key via `SMSINFOBIP_API_KEY` in the environment (do not put secrets in config files). Update `conf/docker/config_docker.json` to use this provider.

```json
{
  "smsProvider": "smsinfobip",
  "smsProviderUrl": "https://api.example.com/sms/send",
  "smsProviderSenderId": "your-sender",
  "smsProviderConnectTimeoutSeconds": 2,
  "smsProviderTimeoutSeconds": 30
}
```

## push gateway (FCM)

Set `PUSH_FCM_CREDENTIAL_JSON_PATH` (preferred) or `PUSH_FCM_BEARER_TOKEN` in the environment (do not put secrets in config files). Even though `PUSH_FCM_CREDENTIAL_JSON_PATH` is a file path, keep it in env so the secret stays outside config and the path can vary per runtime. Optional: `PUSH_FCM_SCOPE_URL` overrides the default scope. Set `PUSH_FCM_DEBUG=true` to log the FCM error response body (truncated) for non-2xx responses while debugging. Update `conf/docker/config_push_docker.json` to use this provider.

```json
{
  "pushProvider": "fcm",
  "pushProviderUrl": "https://fcm.googleapis.com/v1/projects/enc-scb/messages:send",
  "pushProviderConnectTimeoutSeconds": 2,
  "pushProviderTimeoutSeconds": 30,
  "grafanaDashboardUrl": "http://localhost:3000/d/gateway-overview-push"
}
```

## Final note

Gateway is a real-time submission bridge.
It guarantees truthful submission outcomes and nothing beyond that.
Any capability beyond submission (delivery tracking, retries, reconciliation)
requires a separate system with explicit time ownership.
