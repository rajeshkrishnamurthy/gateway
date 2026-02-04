# Setu capabilities (high-level)

This document summarizes Setu's implemented capabilities at a high level for internal communication. It is intentionally concise and does not list the detailed nuances, limits, or exclusions of each capability.

## SubmissionManager

- Multiple SubmissionManager instances can receive HTTP requests.
- Leader-lease driven executor model.
- Retry models implemented in SubmissionManager with exhaustion built in.
- Client can initiate an intent with or without `waitSeconds`.
- Returns synchronously after `waitSeconds` or immediately with `pending`.
- GET intent status.
- Callback to webhook on terminal state.

## Gateway and Routing

- Gateway routing based on submissionTarget.
- One-shot gateway submission to external providers.
- Multiple gateway instances managed by HAProxy.

## Adapters and Providers

- Adapter-driven integration with external providers.

## Persistence

- Persistence of intents, attempts, and webhook deliveries.

## Secrets

- Secrets maintained via environment variables.
- JWT tokens for FCM implemented through json file. 

## Observability

- Prometheus metrics for gateway and SubmissionManager.
- Grafana dashboards for operational visibility.

## Testing

- Unit tests with ~70% coverage.
- 10 integration test scenarios that pass.

## Fake providers (local-only)

- Fake provider servers for smoke tests and demos: `fakeprovider`, `modelprovider`, `sms24x7provider`, `smsinfobipprovider`, `smskarixprovider`, `webhooksink`.

## Tooling and Operations

- Development environment set up in Docker Compose.
- Command Center view for service health and on/off status.
