# Setu setup (developer quickstart)

This is the single-page setup guide for new developers.

## Prerequisites
- Docker Desktop (required for the default dev environment)
- Go toolchain (required if you run services outside Docker)

## One-time setup
1) Create the local SQL Server env file:
```
mkdir -p backend
cat > backend/.env <<'EOF'
ACCEPT_EULA=Y
MSSQL_PID=Developer
MSSQL_SA_PASSWORD=CHANGEME_STRONG_PASSWORD
EOF
```

2) (Optional) Push gateway credentials
- The Compose dev path uses a Firebase service account JSON:
  - File: `backend/conf/firebase.json`
  - Docker Compose sets `PUSH_FCM_CREDENTIAL_JSON_PATH=/app/backend/conf/firebase.json`
- Alternative (optional): use a bearer token
  - Env: `PUSH_FCM_BEARER_TOKEN`
  - Optional scope override: `PUSH_FCM_SCOPE_URL`

3) (Optional) SMS provider API keys  
Only needed if you change `conf/docker/config_docker.json` away from `smsProvider: "model"`:
- `SMS24X7_API_KEY`
- `SMSKARIX_API_KEY`
- `SMSINFOBIP_API_KEY`

4) (Optional) Webhook secrets  
The Compose registry (`backend/conf/submission/submission_targets_docker.json`) sets:
```
"allowUnsignedWebhooks": true
```
If you flip this to `false`, each target webhook must set `secretEnv` and you must export that env var for `submission-manager`.

## Start everything (Docker Compose)
From the repo root:
```
docker compose up -d
```

## Stop everything
```
docker compose down
```

## Key env variables (Compose defaults)
The following are wired by Compose and usually do not need manual changes:
- `MSSQL_HOST=mssql`
- `MSSQL_PORT=1433`
- `MSSQL_USER=sa`
- `MSSQL_DATABASE=setu`
- `MSSQL_ENCRYPT=disable`
- `PUSH_FCM_CREDENTIAL_JSON_PATH=/app/backend/conf/firebase.json`

## Notes
- `backend/.env` is intentionally untracked; each developer should create their own.
- The Compose SQL Server image runs with `platform: linux/amd64` for macOS compatibility.
