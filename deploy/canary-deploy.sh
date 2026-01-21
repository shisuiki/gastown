#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR=$(git rev-parse --show-toplevel 2>/dev/null || pwd)
CONTAINER_NAME=${CONTAINER_NAME:-gastown-canary}
CANARY_PORT=${CANARY_PORT:-8081}
GT_ROOT=${GT_ROOT:-/home/shisui/gt}
GASTOWN_REF=${GASTOWN_REF:-}
ENV_CONFIG_REF=${ENV_CONFIG_REF:-}
ENV_CONFIG_DIR=${ENV_CONFIG_DIR:-}
STATE_DIR=${CANARY_STATE_DIR:-"$GT_ROOT/logs"}
STATE_JSON=${CANARY_STATE_JSON:-"$STATE_DIR/canary-deploy.json"}
STATE_ENV=${CANARY_STATE_ENV:-"$STATE_DIR/canary-deploy.env"}
VALIDATE_CANARY=${VALIDATE_CANARY:-1}
VALIDATION_SCRIPT=${VALIDATION_SCRIPT:-"$ROOT_DIR/deploy/canary-validate.sh"}

log() {
  printf "[%s] %s\n" "$(date -u +%Y-%m-%dT%H:%M:%SZ)" "$*"
}

fail() {
  log "ERROR: $*"
  exit 1
}

require_dir() {
  local dir=$1
  if [ ! -d "$dir" ]; then
    fail "missing directory: $dir"
  fi
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
  fail "docker daemon unavailable or permission denied. Add user to docker group or enable passwordless sudo for docker."
fi

if [ -z "$GASTOWN_REF" ]; then
  if command -v git >/dev/null 2>&1; then
    GASTOWN_REF=$(git -C "$ROOT_DIR" rev-parse HEAD)
  else
    GASTOWN_REF="unknown"
  fi
fi

if [ -z "$ENV_CONFIG_REF" ] && [ -n "$ENV_CONFIG_DIR" ]; then
  if command -v git >/dev/null 2>&1; then
    ENV_CONFIG_REF=$(git -C "$ENV_CONFIG_DIR" rev-parse HEAD)
  fi
fi

require_dir "$GT_ROOT"

if [ -z "${GT_WEB_AUTH_TOKEN:-}" ]; then
  fail "GT_WEB_AUTH_TOKEN is required for canary deployment"
fi

IMAGE_TAG="gastown:canary-${GASTOWN_REF:0:12}"

log "Using docker command: $DOCKER_CMD"
log "Building image: $IMAGE_TAG"
$DOCKER_CMD build -t "$IMAGE_TAG" "$ROOT_DIR"

PREVIOUS_IMAGE=""
if $DOCKER_CMD inspect "$CONTAINER_NAME" >/dev/null 2>&1; then
  PREVIOUS_IMAGE=$($DOCKER_CMD inspect --format '{{.Config.Image}}' "$CONTAINER_NAME" 2>/dev/null || true)
  log "Found existing container $CONTAINER_NAME (image: ${PREVIOUS_IMAGE:-unknown})"
  $DOCKER_CMD rm -f "$CONTAINER_NAME"
fi

ROLLBACK_ENABLED=0
if [ -n "$PREVIOUS_IMAGE" ]; then
  ROLLBACK_ENABLED=1
fi

rollback() {
  if [ "$ROLLBACK_ENABLED" -eq 1 ]; then
    log "Attempting rollback to $PREVIOUS_IMAGE"
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
  else
    log "Rollback skipped (no previous image)"
  fi
}

on_error() {
  log "Deploy failed; invoking rollback"
  rollback
}

trap on_error ERR

log "Starting canary container $CONTAINER_NAME on port $CANARY_PORT"
$DOCKER_CMD run -d \
  --name "$CONTAINER_NAME" \
  --restart=always \
  -p "$CANARY_PORT:8080" \
  -v "$GT_ROOT:/gt" \
  -e GT_WEB_AUTH_TOKEN="$GT_WEB_AUTH_TOKEN" \
  -e GT_WEB_ALLOW_REMOTE=1 \
  --label "gastown_ref=$GASTOWN_REF" \
  --label "env_config_ref=$ENV_CONFIG_REF" \
  "$IMAGE_TAG" >/dev/null

if [ -n "${GASTOWN_MIGRATIONS_CMD:-}" ]; then
  log "Running migrations: $GASTOWN_MIGRATIONS_CMD"
  $DOCKER_CMD exec "$CONTAINER_NAME" sh -c "$GASTOWN_MIGRATIONS_CMD"
else
  log "No migrations command specified (GASTOWN_MIGRATIONS_CMD empty)"
fi

log "Waiting for container health"
for _ in $(seq 1 12); do
  status=$($DOCKER_CMD inspect --format '{{.State.Health.Status}}' "$CONTAINER_NAME" 2>/dev/null || echo "unknown")
  if [ "$status" = "healthy" ]; then
    log "Container is healthy"
    break
  fi
  if [ "$status" = "unhealthy" ]; then
    fail "Container reported unhealthy"
  fi
  sleep 5
  if [ "$status" = "unknown" ]; then
    log "Health status unavailable yet"
  fi
done

if [ "$VALIDATE_CANARY" != "0" ]; then
  if [ -x "$VALIDATION_SCRIPT" ]; then
    log "Running canary validation"
    "$VALIDATION_SCRIPT" --container "$CONTAINER_NAME" --port "$CANARY_PORT"
  else
    fail "validation script missing or not executable: $VALIDATION_SCRIPT"
  fi
else
  log "Skipping validation (VALIDATE_CANARY=0)"
fi

mkdir -p "$STATE_DIR"
cat <<META > "$STATE_JSON"
{
  "deployed_at": "$(date -u +%Y-%m-%dT%H:%M:%SZ)",
  "host": "$(hostname)",
  "container": "$CONTAINER_NAME",
  "image": "$IMAGE_TAG",
  "previous_image": "$PREVIOUS_IMAGE",
  "gastown_ref": "$GASTOWN_REF",
  "env_config_ref": "$ENV_CONFIG_REF",
  "canary_port": "$CANARY_PORT"
}
META

cat <<META > "$STATE_ENV"
CURRENT_IMAGE="$IMAGE_TAG"
PREVIOUS_IMAGE="$PREVIOUS_IMAGE"
GASTOWN_REF="$GASTOWN_REF"
ENV_CONFIG_REF="$ENV_CONFIG_REF"
CONTAINER_NAME="$CONTAINER_NAME"
CANARY_PORT="$CANARY_PORT"
META

log "Deployment metadata written to $STATE_JSON"
trap - ERR
log "Canary deploy complete"
