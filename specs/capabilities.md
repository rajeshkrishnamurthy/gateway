# Setu capabilities (high-level)

This document summarizes Setu's implemented capabilities at a high level for internal communication. It is intentionally concise and does not list the detailed nuances, limits, or exclusions of each capability.

1. Client can initiate an intent with or without `waitSeconds`.
2. Multiple SubmissionManager instances can receive HTTP requests.
3. Leader-lease driven executor model (future work).
4. Returns synchronously after `waitSeconds` or immediately with `pending`.
5. GET intent status.
6. Callback to webhook on terminal state.
7. Gateway routing based on submissionTarget.
8. Adapter-driven integration with external providers.
9. Retry models implemented in SubmissionManager with exhaustion built in.
10. One-shot gateway submission to external providers.
11. Secrets maintained via environment variables.
12. Persistence of intents, attempts, and webhook deliveries.
13. Prometheus metrics for gateway and SubmissionManager.
14. Multiple gateway instances managed by HAProxy.
15. Grafana dashboards for operational visibility.
16. Unit tests with ~70% coverage.
17. 10 integration test scenarios that pass.
18. Development environment set up in Docker Compose.
19. Command Center view for service health and on/off status.
