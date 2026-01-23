#!/bin/bash
#
# entrypoint-full.sh - Full Gas Town container entrypoint
#
# Starts mayor daemon, deacon, and optional web UI in orchestrated sequence.
# Designed for canary testing with full agent infrastructure.
#

set -euo pipefail

log() {
    echo "[$(date -u +%Y-%m-%dT%H:%M:%SZ)] [entrypoint] $*"
}

wait_for_daemon() {
    local max_wait=${1:-60}
    local waited=0
    log "Waiting for daemon (max ${max_wait}s)..."
    while [ $waited -lt $max_wait ]; do
        if gt daemon status >/dev/null 2>&1; then
            log "Daemon ready after ${waited}s"
            return 0
        fi
        sleep 1
        ((waited++))
    done
    log "ERROR: Daemon not ready after ${max_wait}s"
    return 1
}

wait_for_session() {
    local session=$1
    local max_wait=${2:-30}
    local waited=0
    log "Waiting for session $session (max ${max_wait}s)..."
    while [ $waited -lt $max_wait ]; do
        if tmux has-session -t "$session" 2>/dev/null; then
            log "Session $session ready after ${waited}s"
            return 0
        fi
        sleep 1
        ((waited++))
    done
    log "ERROR: Session $session not ready after ${max_wait}s"
    return 1
}

start_daemon() {
    log "Starting gt daemon..."
    gt daemon start || {
        log "Daemon start returned non-zero, checking status..."
        sleep 2
        if gt daemon status >/dev/null 2>&1; then
            log "Daemon is running despite start error"
            return 0
        fi
        return 1
    }
}

start_deacon() {
    log "Starting deacon..."
    gt deacon start || {
        log "Deacon start command completed"
    }
    wait_for_session "hq-deacon" 30
}

start_web_ui() {
    log "Starting web UI on port 8080..."
    # Run in foreground to keep container alive
    exec gt gui --port 8080
}

run_full_mode() {
    log "=== Starting Full Gas Town Instance ==="
    log "GT_ROOT: ${GT_ROOT:-/gt}"
    log "HOME: ${HOME:-/home/gastown}"

    # Ensure we're in GT_ROOT
    cd "${GT_ROOT:-/gt}"

    # Fix git safe.directory for mounted volume
    log "Configuring git safe.directory..."
    git config --global --add safe.directory /gt || true

    # Initialize beads if needed
    if [ -d "/gt/.beads" ] && [ ! -f "/home/gastown/.config/beads/config.json" ]; then
        log "Initializing beads..."
        mkdir -p /home/gastown/.config/beads
        bd init /gt/.beads 2>/dev/null || true
    fi

    # Initialize tmux server
    log "Initializing tmux server..."
    tmux start-server || true

    # Start daemon first
    start_daemon
    wait_for_daemon 60

    # Start deacon (handles patrol, witness/refinery management)
    start_deacon

    # Give deacon time to initialize patrol
    log "Allowing deacon patrol initialization..."
    sleep 5

    # Show status
    log "=== Gas Town Status ==="
    gt status || true

    log "=== Full Gas Town Ready ==="

    # Start web UI in foreground (keeps container alive)
    start_web_ui
}

run_gui_only() {
    log "Starting GUI-only mode..."
    exec gt gui --port 8080
}

# Main
MODE="${1:-full}"

case "$MODE" in
    full)
        run_full_mode
        ;;
    gui)
        run_gui_only
        ;;
    *)
        log "Unknown mode: $MODE"
        log "Usage: entrypoint.sh [full|gui]"
        exit 1
        ;;
esac
