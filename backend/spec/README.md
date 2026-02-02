# Backend specs

This directory contains canonical, as-is specifications for backend behavior. These docs define system semantics and constraints that code must follow. If behavior changes, update the relevant spec in the same change.

## Index

- `submission-manager.md` - SubmissionIntent, submissionTarget contracts, and SubmissionManager semantics.
- `submission-manager-metrics.md` - Prometheus metrics for SubmissionManager.
- `manager-sync-timeout.md` - Sync wait behavior for POST /v1/intents.
- `vision.md` - Setu vision, goals, and scope.
- `gateway-contracts.md` - SMS and push gateway contracts, HTTP endpoints, and submission-only behavior.
- `gateway-metrics.md` - Prometheus metrics emitted by gateways and their meanings.
- `gateway-configs.md` - Gateway config file schemas and validation rules.
- `admin-portal.md` - Admin portal config and proxy behavior.
- `services-health.md` - Command Center config, health checks, and start/stop behavior.
- `model-provider-adapter.md` - Canonical model SMS provider adapter spec.
