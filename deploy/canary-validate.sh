#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR=$(git rev-parse --show-toplevel 2>/dev/null || pwd)
CONTAINER_NAME=${CONTAINER_NAME:-gastown-canary}
CANARY_PORT=${CANARY_PORT:-8081}
GT_ROOT=${GT_ROOT:-/home/shisui/gt}
LOG_TAIL_LINES=${LOG_TAIL_LINES:-200}

usage() {
  cat <<'USAGE'
Usage: canary-validate.sh [options]

Options:
  --container <name>   Container name (default: gastown-canary)
  --port <port>        Canary port (default: 8081)
  --log-tail <lines>   Log lines to inspect (default: 200)
  -h, --help           Show help

Env:
  GT_WEB_AUTH_TOKEN    Required for /api/version check
  GT_DB_CHECK_CMD      Optional DB check command (exec inside container)
USAGE
}

while [ $# -gt 0 ]; do
  case "$1" in
    --container) CONTAINER_NAME="$2"; shift 2 ;;
    --port) CANARY_PORT="$2"; shift 2 ;;
    --log-tail) LOG_TAIL_LINES="$2"; shift 2 ;;
    -h|--help) usage; exit 0 ;;
    *) echo "Unknown option: $1" >&2; usage; exit 2 ;;
  esac
done

log() {
  printf "[%s] %s\n" "$(date -u +%Y-%m-%dT%H:%M:%SZ)" "$*"
}

fail() {
  log "ERROR: $*"
  exit 1
}

detect_docker() {
  if docker info >/dev/null 2>&1; then
    echo "docker"
    return 0
  fi

  if sudo -n docker info >/dev/null 2>&1; then
    echo "sudo docker"
    return 0
  fi

  return 1
}

DOCKER_CMD=$(detect_docker || true)
if [ -z "$DOCKER_CMD" ]; then
  fail "docker daemon unavailable or permission denied"
fi

if ! $DOCKER_CMD inspect "$CONTAINER_NAME" >/dev/null 2>&1; then
  fail "container not found: $CONTAINER_NAME"
fi

log "Checking container state"
state=$($DOCKER_CMD inspect --format '{{.State.Status}}' "$CONTAINER_NAME" 2>/dev/null || echo "unknown")
if [ "$state" != "running" ]; then
  fail "container not running (state=$state)"
fi

health=$($DOCKER_CMD inspect --format '{{.State.Health.Status}}' "$CONTAINER_NAME" 2>/dev/null || echo "unknown")
if [ "$health" = "unhealthy" ]; then
  fail "container reported unhealthy"
fi

exec_in_container() {
  $DOCKER_CMD exec \
    -e GT_WEB_AUTH_TOKEN="${GT_WEB_AUTH_TOKEN:-}" \
    "$CONTAINER_NAME" \
    sh -c "$1"
}

log "Validating binaries"
exec_in_container "gt version >/dev/null"
exec_in_container "bd version >/dev/null"

if [ -z "${GT_WEB_AUTH_TOKEN:-}" ]; then
  fail "GT_WEB_AUTH_TOKEN is required for API validation"
fi

log "Validating API endpoint"
exec_in_container '
if command -v curl >/dev/null 2>&1; then
  curl -fsS -H "Authorization: Bearer ${GT_WEB_AUTH_TOKEN}" http://localhost:8080/api/version >/dev/null
elif command -v wget >/dev/null 2>&1; then
  wget -qO- --header="Authorization: Bearer ${GT_WEB_AUTH_TOKEN}" http://localhost:8080/api/version >/dev/null
else
  echo "Missing curl/wget in container" >&2
  exit 1
fi'

log "Checking required log patterns"
if command -v rg >/dev/null 2>&1; then
  if ! $DOCKER_CMD logs --tail "$LOG_TAIL_LINES" "$CONTAINER_NAME" 2>/dev/null | rg -q "Gas Town GUI starting"; then
    fail "expected log pattern not found: Gas Town GUI starting"
  fi
else
  if ! $DOCKER_CMD logs --tail "$LOG_TAIL_LINES" "$CONTAINER_NAME" 2>/dev/null | grep -q "Gas Town GUI starting"; then
    fail "expected log pattern not found: Gas Town GUI starting"
  fi
fi

if [ -n "${GT_DB_CHECK_CMD:-}" ]; then
  log "Running DB check: $GT_DB_CHECK_CMD"
  exec_in_container "$GT_DB_CHECK_CMD"
else
  log "Skipping DB check (GT_DB_CHECK_CMD not set)"
fi

log "Validation passed"
