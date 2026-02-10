# Backend specs

This directory contains canonical, as-is specifications for backend behavior. These docs define system semantics and constraints that code must follow. If behavior changes, update the relevant spec in the same change.

## Index

- `submission-tracking/submission-manager.md` - SubmissionIntent, submissionTarget contracts, and SubmissionManager semantics.
- `submission-tracking/submission-manager-metrics.md` - Prometheus metrics for SubmissionManager.
- `submission-tracking/submission-manager-webhooks.md` - Terminal status webhook callbacks for SubmissionManager.
- `manager-sync-timeout.md` - Sync wait behavior for POST /v1/intents.
- `capabilities.md` - High-level implemented capability list.
- `vision.md` - Setu vision, goals, and scope.
- `gateway-contracts.md` - SMS and push gateway contracts, HTTP endpoints, and submission-only behavior.
- `gateway-metrics.md` - Prometheus metrics emitted by gateways and their meanings.
- `gateway-configs.md` - Gateway config file schemas and validation rules.
- `admin-portal.md` - Admin portal config and proxy behavior.
- `services-health.md` - Command Center config, health checks, and start/stop behavior.
- `model-provider-adapter.md` - Canonical model SMS provider adapter spec.
- `delivery-tracking/delivery-tracking-v1.md` - Delivery tracking V1 umbrella scope and implementation boundary.
- `delivery-tracking/overview.md` - Delivery tracking high-level intent and module-level invariants.
- `delivery-tracking/delivery-status-model.md` - Canonical delivery status and delivery history semantics.
- `delivery-tracking/delivery-freshness.md` - Delivery freshness semantics and staleness behavior.
- `delivery-tracking/intent-correlation.md` - Canonical provider delivery signal to intent correlation contract.
- `delivery-tracking/ubiquitous-language.md` - Canonical terminology for delivery-tracking specs.
- `delivery-tracking/delivery-tracking-v1-core-processing.md` - V1 slice for deterministic delivery-status and delivery-freshness core processing.
- `delivery-tracking/delivery-tracking-v1-webhook-ingestion.md` - V1 slice for provider signal webhook ingestion (trusted-ingress-only in this round).
- `delivery-tracking/delivery-tracking-v1-get-retrieval.md` - V1 slice for delivery read API retrieval contracts.
- `delivery-tracking/delivery-tracking-v1-observability.md` - V1 slice for delivery metrics, logs, and audit visibility.
