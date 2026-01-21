#!/usr/bin/env bash
set -euo pipefail

CONTAINER_NAME=${CONTAINER_NAME:-gastown-canary}
CANARY_PORT=${CANARY_PORT:-8081}
GT_ROOT=${GT_ROOT:-/home/shisui/gt}
STATE_DIR=${CANARY_STATE_DIR:-"$GT_ROOT/logs"}
STATE_ENV=${CANARY_STATE_ENV:-"$STATE_DIR/canary-deploy.env"}

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
  fail "docker daemon unavailable or permission denied."
fi

if [ ! -f "$STATE_ENV" ]; then
  fail "missing state file: $STATE_ENV"
fi

# shellcheck disable=SC1090
source "$STATE_ENV"

if [ -z "${PREVIOUS_IMAGE:-}" ]; then
  fail "PREVIOUS_IMAGE not set in $STATE_ENV"
fi

if [ -z "${GT_WEB_AUTH_TOKEN:-}" ]; then
  fail "GT_WEB_AUTH_TOKEN is required for rollback"
fi

log "Rolling back canary container to $PREVIOUS_IMAGE"
$DOCKER_CMD rm -f "$CONTAINER_NAME" >/dev/null 2>&1 || true
$DOCKER_CMD run -d \
  --name "$CONTAINER_NAME" \
  --restart=always \
  -p "$CANARY_PORT:8080" \
  -v "$GT_ROOT:/gt" \
  -e GT_WEB_AUTH_TOKEN="$GT_WEB_AUTH_TOKEN" \
  -e GT_WEB_ALLOW_REMOTE=1 \
  "$PREVIOUS_IMAGE" >/dev/null

log "Rollback complete"
