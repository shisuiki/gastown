#!/usr/bin/env bash
#
# canary-deploy.sh - Deploy Gas Town canary instance
#
# Deploys gastown-canary with full agent infrastructure:
# - Mayor daemon
# - Deacon (patrol mode)
# - Web UI
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
# Codex credentials directory - persists login across container restarts
CODEX_CREDS_DIR=${CODEX_CREDS_DIR:-/home/shisui/.codex-canary}
# Proxy configuration for Claude/Codex API access (Clash on HK server)
# Force host.docker.internal for container proxy - don't inherit 127.0.0.1 from host
# which would point to container's localhost instead of host's proxy
if echo "$HTTP_PROXY" | grep -q "127.0.0.1"; then
    HTTP_PROXY="http://host.docker.internal:7890"
fi
if echo "$HTTPS_PROXY" | grep -q "127.0.0.1"; then
    HTTPS_PROXY="http://host.docker.internal:7890"
fi
HTTP_PROXY=${HTTP_PROXY:-http://host.docker.internal:7890}
HTTPS_PROXY=${HTTPS_PROXY:-http://host.docker.internal:7890}
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
# Searches multiple locations for unexpired OAuth tokens
setup_claude_creds() {
    log "Setting up Claude credentials..."

    # Take ownership first (may be owned by 10001 from previous deploy)
    sudo chown -R "$(id -u):$(id -g)" "$CLAUDE_CREDS_DIR" 2>/dev/null || true
    mkdir -p "$CLAUDE_CREDS_DIR"

    # Use refresh script if available (searches all credential locations)
    local refresh_script="${GT_ROOT:-/home/shisui/gt}/scripts/refresh-canary-credentials.sh"
    if [ -x "$refresh_script" ]; then
        log "Using credential refresh script..."
        if "$refresh_script" "$CLAUDE_CREDS_DIR"; then
            log "Credentials refreshed via script"
        else
            log "WARNING: Credential refresh script failed"
        fi
    else
        # Fallback: search common credential locations for unexpired token
        log "Searching for unexpired OAuth credentials..."
        local now_ms=$(($(date +%s) * 1000))
        local best_file=""
        local best_expiry=0

        for cred_file in \
            "$HOME/.claude/.credentials.json" \
            "$HOME/.claude-accounts"/*/.credentials.json \
            "$HOME/.claude-"*/.credentials.json; do

            [ -f "$cred_file" ] || continue

            local expiry
            expiry=$(jq -r '.claudeAiOauth.expiresAt // 0' "$cred_file" 2>/dev/null)

            # Check if token is valid (expires > 5 min from now)
            if [ "$expiry" -gt "$((now_ms + 300000))" ] && [ "$expiry" -gt "$best_expiry" ]; then
                best_file="$cred_file"
                best_expiry="$expiry"
            fi
        done

        if [ -n "$best_file" ]; then
            cp "$best_file" "$CLAUDE_CREDS_DIR/.credentials.json"
            local expiry_human
            expiry_human=$(date -d "@$((best_expiry / 1000))" "+%Y-%m-%d %H:%M:%S UTC" 2>/dev/null || echo "unknown")
            log "Copied credentials from: $best_file (expires: $expiry_human)"
        else
            log "WARNING: No unexpired Claude credentials found"
            log "         Mayor session will fail until manually authenticated"
        fi
    fi

    # CRITICAL: chown to container user uid 10001 so container can write
    sudo chown -R 10001:10001 "$CLAUDE_CREDS_DIR" || {
        log "WARNING: Could not chown $CLAUDE_CREDS_DIR to 10001 (need sudo)"
        log "         Container may have write permission issues"
    }
}

setup_claude_creds

# Set up Codex credentials directory for container
# Always sync fresh credentials from host (tokens may expire)
setup_codex_creds() {
    # Take ownership first (may be owned by 10001 from previous deploy)
    sudo chown -R "$(id -u):$(id -g)" "$CODEX_CREDS_DIR" 2>/dev/null || true
    mkdir -p "$CODEX_CREDS_DIR"
    # Always copy fresh credentials if available
    if [ -d "$HOME/.codex" ]; then
        cp "$HOME/.codex/auth.json" "$CODEX_CREDS_DIR/" 2>/dev/null || true
        cp "$HOME/.codex/config.toml" "$CODEX_CREDS_DIR/" 2>/dev/null || true
        log "Synced Codex credentials from host"
    else
        log "WARNING: No Codex credentials found at $HOME/.codex"
        log "         Run 'codex auth login' on host before deploying"
    fi
    # CRITICAL: chown to container user uid 10001 so container can write
    sudo chown -R 10001:10001 "$CODEX_CREDS_DIR" || {
        log "WARNING: Could not chown $CODEX_CREDS_DIR to 10001 (need sudo)"
        log "         Codex will report 'Permission denied' errors"
    }
}

setup_codex_creds

IMAGE_TAG="gastown:canary-${GASTOWN_REF:0:12}"

log "=== Gas Town Canary Deployment ==="
log "Using docker: $DOCKER_CMD"
log "GT_ROOT: $GT_ROOT"
log "GTRuntime ref: $GTRUNTIME_REF"
log "Gastown ref: $GASTOWN_REF"

# Build image
log "Building image: $IMAGE_TAG"
$DOCKER_CMD build -t "$IMAGE_TAG" "$ROOT_DIR"

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
            --add-host=host.docker.internal:host-gateway \
            -p "$CANARY_PORT:8080" \
            -v "$GT_ROOT:/gt" \
            -v "$CLAUDE_CREDS_DIR:/home/gastown/.claude" \
            -v "$CODEX_CREDS_DIR:/home/gastown/.codex" \
            -e GT_WEB_AUTH_TOKEN="$GT_WEB_AUTH_TOKEN" \
            -e GT_WEB_ALLOW_REMOTE=1 \
            -e GT_MAIL_BRIDGE_HOST_URL="http://host.docker.internal:8080" \
            -e GT_ENV=canary \
            -e GT_ROOT=/gt \
            "$PREVIOUS_IMAGE" >/dev/null
        log "Rollback complete"
    fi
}

trap rollback ERR

# Start new container
log "Starting canary container on port $CANARY_PORT"
# Build volume mounts - include .claude.json if it was synced
CLAUDE_JSON_MOUNT=""
if [ -f "$CLAUDE_CREDS_DIR/claude.json" ]; then
    CLAUDE_JSON_MOUNT="-v $CLAUDE_CREDS_DIR/claude.json:/home/gastown/.claude.json"
fi
$DOCKER_CMD run -d \
    --name "$CONTAINER_NAME" \
    --restart=always \
    --add-host=host.docker.internal:host-gateway \
    -p "$CANARY_PORT:8080" \
    -v "$GT_ROOT:/gt" \
    -v "$CLAUDE_CREDS_DIR:/home/gastown/.claude" \
    $CLAUDE_JSON_MOUNT \
    -v "$CODEX_CREDS_DIR:/home/gastown/.codex" \
    -e GT_WEB_AUTH_TOKEN="$GT_WEB_AUTH_TOKEN" \
    -e GT_WEB_ALLOW_REMOTE=1 \
    -e GT_MAIL_BRIDGE_HOST_URL="http://host.docker.internal:8080" \
    -e GT_ENV=canary \
    -e GT_ROOT=/gt \
    -e HTTP_PROXY="$HTTP_PROXY" \
    -e HTTPS_PROXY="$HTTPS_PROXY" \
    -e http_proxy="$HTTP_PROXY" \
    -e https_proxy="$HTTPS_PROXY" \
    -e NO_PROXY="localhost,127.0.0.1,host.docker.internal" \
    --label "gastown_ref=$GASTOWN_REF" \
    --label "gtruntime_ref=$GTRUNTIME_REF" \
    --label "deploy_mode=standard" \
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
    "deploy_mode": "standard"
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
DEPLOY_MODE="standard"
META

trap - ERR
log "=== Canary Deploy Complete ==="
log "Container: $CONTAINER_NAME"
log "Port: $CANARY_PORT"
log "Mode: standard (daemon + deacon + web)"
