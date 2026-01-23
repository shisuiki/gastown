#!/usr/bin/env bash
#
# canary-deploy-full.sh - Deploy full Gas Town canary instance
#
# Deploys gastown-canary with full agent infrastructure:
# - Mayor daemon
# - Deacon (patrol mode)
# - Web UI
#
# Uses Dockerfile.full instead of standard Dockerfile
#
set -euo pipefail

ROOT_DIR=$(git rev-parse --show-toplevel 2>/dev/null || pwd)
CONTAINER_NAME=${CONTAINER_NAME:-gastown-canary}
CANARY_PORT=${CANARY_PORT:-8081}
GT_ROOT=${GT_ROOT:-/home/shisui/gt-canary}
GASTOWN_REF=${GASTOWN_REF:-}
GTRUNTIME_REF=${GTRUNTIME_REF:-}
STATE_DIR=${CANARY_STATE_DIR:-"$GT_ROOT/logs"}
# Claude credentials directory - persists login across container restarts
CLAUDE_CREDS_DIR=${CLAUDE_CREDS_DIR:-/home/shisui/.claude-canary}
STATE_JSON=${CANARY_STATE_JSON:-"$STATE_DIR/canary-deploy.json"}
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
    fail "docker daemon unavailable or permission denied"
fi

# Capture refs
if [ -z "$GASTOWN_REF" ]; then
    GASTOWN_REF=$(git -C "$ROOT_DIR" rev-parse HEAD 2>/dev/null || echo "unknown")
fi

if [ -z "$GTRUNTIME_REF" ]; then
    GTRUNTIME_REF=$(git -C "$GT_ROOT" rev-parse HEAD 2>/dev/null || echo "unknown")
fi

# Validate canary workspace
if [ ! -d "$GT_ROOT/.beads/formulas" ]; then
    fail "Canary workspace missing .beads/formulas: $GT_ROOT"
fi

if [ -z "${GT_WEB_AUTH_TOKEN:-}" ]; then
    fail "GT_WEB_AUTH_TOKEN is required"
fi

# Set up Claude credentials directory for container
# This persists login across container restarts
setup_claude_creds() {
    if [ ! -d "$CLAUDE_CREDS_DIR" ]; then
        log "Creating Claude credentials directory: $CLAUDE_CREDS_DIR"
        mkdir -p "$CLAUDE_CREDS_DIR"
        # Copy existing credentials if available
        if [ -f "$HOME/.claude/.credentials.json" ]; then
            cp "$HOME/.claude/.credentials.json" "$CLAUDE_CREDS_DIR/"
            log "Copied existing Claude credentials"
        fi
    fi
    # Ensure directory is accessible by container user (uid 10001)
    chmod 755 "$CLAUDE_CREDS_DIR"
    if [ -f "$CLAUDE_CREDS_DIR/.credentials.json" ]; then
        chmod 644 "$CLAUDE_CREDS_DIR/.credentials.json"
    fi
}

setup_claude_creds

IMAGE_TAG="gastown:canary-full-${GASTOWN_REF:0:12}"

log "=== Full Gas Town Canary Deployment ==="
log "Using docker: $DOCKER_CMD"
log "GT_ROOT: $GT_ROOT"
log "GTRuntime ref: $GTRUNTIME_REF"
log "Gastown ref: $GASTOWN_REF"

# Build with Dockerfile.full
log "Building full image: $IMAGE_TAG"
$DOCKER_CMD build -f Dockerfile.full -t "$IMAGE_TAG" "$ROOT_DIR"

# Stop existing container
PREVIOUS_IMAGE=""
if $DOCKER_CMD inspect "$CONTAINER_NAME" >/dev/null 2>&1; then
    PREVIOUS_IMAGE=$($DOCKER_CMD inspect --format '{{.Config.Image}}' "$CONTAINER_NAME" 2>/dev/null || true)
    log "Stopping existing container (image: ${PREVIOUS_IMAGE:-unknown})"
    $DOCKER_CMD rm -f "$CONTAINER_NAME"
fi

# Rollback function
rollback() {
    if [ -n "$PREVIOUS_IMAGE" ]; then
        log "Rolling back to $PREVIOUS_IMAGE"
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
    fi
}

trap rollback ERR

# Start new container with full mode
log "Starting full canary container on port $CANARY_PORT"
$DOCKER_CMD run -d \
    --name "$CONTAINER_NAME" \
    --restart=always \
    -p "$CANARY_PORT:8080" \
    -v "$GT_ROOT:/gt" \
    -v "$CLAUDE_CREDS_DIR:/home/gastown/.claude" \
    -e GT_WEB_AUTH_TOKEN="$GT_WEB_AUTH_TOKEN" \
    -e GT_WEB_ALLOW_REMOTE=1 \
    -e GT_ROOT=/gt \
    --label "gastown_ref=$GASTOWN_REF" \
    --label "gtruntime_ref=$GTRUNTIME_REF" \
    --label "deploy_mode=full" \
    "$IMAGE_TAG" full

# Wait for health (longer timeout for full startup)
log "Waiting for container health (up to 180s)..."
HEALTH_TIMEOUT=180
for i in $(seq 1 $HEALTH_TIMEOUT); do
    status=$($DOCKER_CMD inspect --format '{{.State.Health.Status}}' "$CONTAINER_NAME" 2>/dev/null || echo "unknown")
    if [ "$status" = "healthy" ]; then
        log "Container healthy after ${i}s"
        break
    fi
    if [ "$status" = "unhealthy" ]; then
        log "Container unhealthy - checking logs..."
        $DOCKER_CMD logs --tail 50 "$CONTAINER_NAME"
        fail "Container reported unhealthy"
    fi
    if [ $((i % 10)) -eq 0 ]; then
        log "Health check status: $status (${i}s elapsed)"
    fi
    sleep 1
done

if [ "$status" != "healthy" ]; then
    log "Container not healthy after ${HEALTH_TIMEOUT}s - checking status..."
    $DOCKER_CMD logs --tail 30 "$CONTAINER_NAME"
    fail "Health check timeout"
fi

# Verify full mode components
log "Verifying full mode components..."
if ! $DOCKER_CMD exec "$CONTAINER_NAME" gt daemon status >/dev/null 2>&1; then
    log "WARNING: Daemon not responding"
fi

if ! $DOCKER_CMD exec "$CONTAINER_NAME" tmux has-session -t hq-deacon 2>/dev/null; then
    log "WARNING: Deacon session not found"
fi

# Show final status
log "=== Container Status ==="
$DOCKER_CMD exec "$CONTAINER_NAME" gt status || true

# Save state
mkdir -p "$STATE_DIR"
cat <<META > "$STATE_JSON"
{
    "deployed_at": "$(date -u +%Y-%m-%dT%H:%M:%SZ)",
    "host": "$(hostname)",
    "container": "$CONTAINER_NAME",
    "image": "$IMAGE_TAG",
    "previous_image": "$PREVIOUS_IMAGE",
    "gastown_ref": "$GASTOWN_REF",
    "gtruntime_ref": "$GTRUNTIME_REF",
    "gt_root": "$GT_ROOT",
    "canary_port": "$CANARY_PORT",
    "deploy_mode": "full"
}
META

cat <<META > "$STATE_ENV"
CURRENT_IMAGE="$IMAGE_TAG"
PREVIOUS_IMAGE="$PREVIOUS_IMAGE"
GASTOWN_REF="$GASTOWN_REF"
GTRUNTIME_REF="$GTRUNTIME_REF"
GT_ROOT="$GT_ROOT"
CONTAINER_NAME="$CONTAINER_NAME"
CANARY_PORT="$CANARY_PORT"
DEPLOY_MODE="full"
META

trap - ERR
log "=== Full Canary Deploy Complete ==="
log "Container: $CONTAINER_NAME"
log "Port: $CANARY_PORT"
log "Mode: full (daemon + deacon + web)"
