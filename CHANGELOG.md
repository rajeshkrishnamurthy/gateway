# Setu changelog

This changelog records user-visible feature additions. History tracking starts on 2026-02-02.

## 2026-02-02

- Added sync wait support for POST /v1/intents via the waitSeconds query parameter.
- Admin portal: added optional waitSeconds input on Test SMS and Test Push to forward sync wait requests to SubmissionManager.
- SubmissionManager: added contract-level terminal webhooks with best-effort single-attempt delivery and unsigned gating via allowUnsignedWebhooks.
- Docker Compose: added webhook-sink service for local webhook testing.
- Dev tooling: added compose integration runner with optional UI and robustness checks.
