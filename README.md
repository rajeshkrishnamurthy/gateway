# Setu

This repo is Docker-only for the MVP to keep dev and devops overhead low.

Start here:
- `backend/spec/vision.md` for Setu vision, goals, and scope.
- `backend/README.md` for gateway contracts and Docker Compose usage.
- `backend/cmd/services-health/README.md` for the Command Center.
- `backend/cmd/admin-portal/README.md` for the Admin Portal.

Local SQL Server (Phase 3a):
- Host: `localhost`
- Port: `1433`
- User: `sa`
- Password: from `backend/.env` (`MSSQL_SA_PASSWORD`)

If/when we move off Docker, see `NON_DOCKER_TODO.md`.
