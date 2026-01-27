# Non-Docker Deployment TODO

This repo is Docker-only for the MVP. If/when we move off Docker, use this checklist.

## Command Center + health
- Update `conf/docker/services_health.json` start/stop commands to use the target runtime (systemd, Windows services, etc.).
- Revisit `healthUrl` values if service base URLs change outside Docker.

## Config + wiring
- Decide on non-Docker config filenames (e.g., `conf/sms/config.json`, `conf/config_push.json`, `conf/admin_portal.json`).
- Update `admin_portal` config so `commandCenterUrl` is not `host.docker.internal`.
- Ensure Grafana datasource URLs point to the correct Prometheus host.

## Infra services
- Define how Grafana/Prometheus/HAProxy are installed and managed without Docker.
- Revisit log levels and persistence paths for Grafana/Prometheus.

## Documentation
- Replace Docker-only instructions with the new deployment path.
- Add platform-specific start/stop examples if needed.
