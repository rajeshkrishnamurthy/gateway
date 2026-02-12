#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

DEFAULT_SOURCE_DIR="/Users/rajeshk/code/go/go-toolkit/setu"
SOURCE_DIR="${SETU_WORKTREE_SOURCE:-}"
RUN_UP=1

usage() {
  cat <<'EOF'
Usage: scripts/worktree-compose-up.sh [--source <path>] [--no-up]

Bootstraps worktree-local Docker prerequisites, then starts the stack.

Options:
  --source <path>  Source checkout to copy local-only files from
                   (defaults to /Users/rajeshk/code/go/go-toolkit/setu when available)
  --no-up          Only run bootstrap checks/copies; do not run docker compose up
  -h, --help       Show this help
EOF
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --source)
      if [[ $# -lt 2 ]]; then
        echo "error: --source requires a path" >&2
        exit 1
      fi
      SOURCE_DIR="$2"
      shift 2
      ;;
    --no-up)
      RUN_UP=0
      shift
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "error: unknown option: $1" >&2
      usage >&2
      exit 1
      ;;
  esac
done

if [[ -z "$SOURCE_DIR" && "$ROOT_DIR" != "$DEFAULT_SOURCE_DIR" && -d "$DEFAULT_SOURCE_DIR" ]]; then
  SOURCE_DIR="$DEFAULT_SOURCE_DIR"
fi

if [[ -n "$SOURCE_DIR" && ! -d "$SOURCE_DIR" ]]; then
  echo "error: source directory does not exist: $SOURCE_DIR" >&2
  exit 1
fi

TARGET_ENV="$ROOT_DIR/backend/.env"
TARGET_FIREBASE="$ROOT_DIR/backend/conf/firebase.json"

copy_if_missing() {
  local source_file="$1"
  local target_file="$2"
  local label="$3"

  if [[ -f "$target_file" ]]; then
    echo "ok: $label exists"
    return 0
  fi

  if [[ -n "$SOURCE_DIR" && -f "$source_file" ]]; then
    mkdir -p "$(dirname "$target_file")"
    cp "$source_file" "$target_file"
    chmod 600 "$target_file" 2>/dev/null || true
    echo "copied: $label from $source_file"
    return 0
  fi

  echo "error: missing $label at $target_file" >&2
  if [[ -n "$SOURCE_DIR" ]]; then
    echo "error: source file not found at $source_file" >&2
  else
    echo "error: provide --source <path> (or SETU_WORKTREE_SOURCE) to copy local files" >&2
  fi
  exit 1
}

if [[ -n "$SOURCE_DIR" ]]; then
  SOURCE_ENV="$SOURCE_DIR/backend/.env"
  SOURCE_FIREBASE="$SOURCE_DIR/backend/conf/firebase.json"
else
  SOURCE_ENV=""
  SOURCE_FIREBASE=""
fi

copy_if_missing "$SOURCE_ENV" "$TARGET_ENV" "backend/.env"
copy_if_missing "$SOURCE_FIREBASE" "$TARGET_FIREBASE" "backend/conf/firebase.json"

required_env_keys=(ACCEPT_EULA MSSQL_PID MSSQL_SA_PASSWORD)
missing_keys=()
for key in "${required_env_keys[@]}"; do
  if ! rg -q "^${key}=.+" "$TARGET_ENV"; then
    missing_keys+=("$key")
  fi
done

if [[ ${#missing_keys[@]} -gt 0 ]]; then
  echo "error: backend/.env is missing required keys: ${missing_keys[*]}" >&2
  exit 1
fi

if rg -q '^MSSQL_SA_PASSWORD=CHANGEME' "$TARGET_ENV"; then
  echo "error: backend/.env still uses a placeholder MSSQL_SA_PASSWORD" >&2
  exit 1
fi

if command -v jq >/dev/null 2>&1; then
  if ! jq -e 'type == "object" and .type == "service_account"' "$TARGET_FIREBASE" >/dev/null; then
    echo "error: backend/conf/firebase.json is not a valid Firebase service account JSON" >&2
    exit 1
  fi
fi

export COMPOSE_PROJECT_NAME="${COMPOSE_PROJECT_NAME:-setu}"
export PUSH_FCM_DEBUG="${PUSH_FCM_DEBUG:-false}"

echo "compose project: $COMPOSE_PROJECT_NAME"
echo "PUSH_FCM_DEBUG: $PUSH_FCM_DEBUG"

if volume_names="$(docker volume ls --format '{{.Name}}' 2>/dev/null)"; then
  if printf '%s\n' "$volume_names" | rg -qx "${COMPOSE_PROJECT_NAME}_mssql-data"; then
    echo "info: found existing ${COMPOSE_PROJECT_NAME}_mssql-data volume; keep MSSQL_SA_PASSWORD consistent across worktrees"
  fi
else
  echo "warn: could not query docker volumes; skipping mssql-data reuse check"
fi

docker compose config >/tmp/"${COMPOSE_PROJECT_NAME}"-compose-config.yaml
echo "ok: docker compose config rendered"

if [[ "$RUN_UP" -eq 0 ]]; then
  echo "bootstrap complete (--no-up)"
  exit 0
fi

docker compose up -d
docker compose ps

echo "done: stack is up. optional Command Center: (cd backend && go run ./cmd/services-health -config conf/docker/services_health.json -addr :8070)"
