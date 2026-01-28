#!/bin/bash
#
# canary-run.sh - Start the canary container with proper mounts
#
# Usage:
#   ./canary-run.sh              # Start with default settings
#   ./canary-run.sh --rebuild    # Rebuild image first
#

set -euo pipefail

# Load config
CONFIG_FILE="/etc/gastown/canary.env"
if [ -f "$CONFIG_FILE" ]; then
    source "$CONFIG_FILE"
fi

# Defaults
CONTAINER_NAME="${CANARY_CONTAINER:-gastown-canary}"
HOST_PORT="${CANARY_PORT:-8081}"
IMAGE="${CANARY_IMAGE:-gastown:canary-latest}"

# Host paths
HOST_GT_DATA="${GT_CANARY_DATA:-$HOME/gt-canary-data}"
HOST_GT_LIVE="${HOST_GT_LIVE:-$HOME/gt}"
HOST_CLAUDE_DIR="$HOME/.claude"
HOST_CLAUDE_CODE_DIR="$HOME/.config/claude-code"

# Proxy settings (for Claude API access)
PROXY_HOST="${PROXY_HOST:-172.17.0.1}"
PROXY_PORT="${PROXY_PORT:-7890}"
PROXY_URL="http://${PROXY_HOST}:${PROXY_PORT}"

log() {
    echo "[canary-run] $*"
}

rebuild_image() {
    local src_dir="${GASTOWN_CANARY_SRC:-$HOME/laplace/gastown-canary-src}"
    local tag="gastown:canary-$(date +%Y%m%d-%H%M)"

    log "Building image from $src_dir..."
    cd "$src_dir"
    docker build -f Dockerfile.canary -t "$tag" .

    # Also tag as latest
    docker tag "$tag" gastown:canary-latest

    log "Built: $tag"
    echo "$tag"
}

stop_existing() {
    if docker ps -q --filter "name=$CONTAINER_NAME" | grep -q .; then
        log "Stopping existing container..."
        docker stop "$CONTAINER_NAME" >/dev/null
    fi
    if docker ps -aq --filter "name=$CONTAINER_NAME" | grep -q .; then
        log "Removing existing container..."
        docker rm "$CONTAINER_NAME" >/dev/null
    fi
}

start_container() {
    local image="$1"

    log "Starting container: $CONTAINER_NAME"
    log "  Image: $image"
    log "  Port: $HOST_PORT -> 8080"
    log "  GT_ROOT: /home/gastown/gt"

    # Run as host user (1000) so Claude credentials are accessible
    docker run -d \
        --name "$CONTAINER_NAME" \
        --restart unless-stopped \
        --user 1000:1000 \
        -p "${HOST_PORT}:8080" \
        -v "${HOST_GT_DATA}:/home/gastown/gt" \
        -v "${HOST_GT_LIVE}:/home/gastown/gt-live:ro" \
        -v "${HOST_CLAUDE_DIR}:/home/gastown/.claude" \
        -v "${HOST_CLAUDE_CODE_DIR}:/home/gastown/.config/claude-code" \
        -e GT_WEB_AUTH_TOKEN="${GT_WEB_AUTH_TOKEN:-}" \
        -e GT_WEB_ALLOW_REMOTE=1 \
        -e GT_ROOT=/home/gastown/gt \
        -e HOME=/home/gastown \
        -e HTTP_PROXY="${PROXY_URL}" \
        -e HTTPS_PROXY="${PROXY_URL}" \
        -e http_proxy="${PROXY_URL}" \
        -e https_proxy="${PROXY_URL}" \
        -e NO_PROXY="localhost,127.0.0.1" \
        "$image" \
        gui --port 8080

    log "Container started"

    # Wait for health
    log "Waiting for container to be healthy..."
    local max_wait=30
    local waited=0
    while [ $waited -lt $max_wait ]; do
        if docker ps --filter "name=$CONTAINER_NAME" --format "{{.Status}}" | grep -q "healthy"; then
            log "Container is healthy"
            return 0
        fi
        sleep 1
        ((waited++))
    done

    log "WARNING: Container may not be healthy after ${max_wait}s"
}

show_status() {
    log "=== Container Status ==="
    docker ps --filter "name=$CONTAINER_NAME" --format "table {{.Names}}\t{{.Status}}\t{{.Ports}}"

    log ""
    log "=== Verify Claude ==="
    docker exec "$CONTAINER_NAME" claude --version 2>/dev/null || echo "Claude not accessible"

    log ""
    log "=== Verify gt ==="
    docker exec "$CONTAINER_NAME" gt version
}

main() {
    local do_rebuild=false

    for arg in "$@"; do
        case "$arg" in
            --rebuild|-r)
                do_rebuild=true
                ;;
        esac
    done

    local image="$IMAGE"

    if [ "$do_rebuild" = true ]; then
        image=$(rebuild_image)
    fi

    stop_existing
    start_container "$image"

    sleep 3
    show_status
}

main "$@"
