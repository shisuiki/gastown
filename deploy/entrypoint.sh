#!/bin/bash
#
# entrypoint.sh - Gas Town container entrypoint
#
# Starts mayor daemon, deacon, and optional web UI in orchestrated sequence.
# Designed for canary testing with full agent infrastructure.
#

set -euo pipefail

log() {
    echo "[$(date -u +%Y-%m-%dT%H:%M:%SZ)] [entrypoint] $*"
}

wait_for_session() {
    local session=$1
    local max_wait=${2:-30}
    local waited=0
    log "Waiting for session $session (max ${max_wait}s)..."
    while [ $waited -lt $max_wait ]; do
        if su-exec gastown tmux has-session -t "$session" 2>/dev/null; then
            log "Session $session ready after ${waited}s"
            return 0
        fi
        sleep 1
        ((waited++))
    done
    log "ERROR: Session $session not ready after ${max_wait}s"
    return 1
}

run_full_mode() {
    log "=== Starting Full Gas Town Instance ==="
    log "GT_ROOT: ${GT_ROOT:-/gt}"
    log "HOME: ${HOME:-/home/gastown}"

    # Ensure we're in GT_ROOT
    cd "${GT_ROOT:-/gt}"

    # Phase 0: Fix permissions (runs as root before user switch)
    log "Phase 0: Fixing ownership for gastown user..."
    chown -R gastown:gastown /gt/mayor 2>/dev/null || true
    chown -R gastown:gastown /gt/.beads 2>/dev/null || true
    chown -R gastown:gastown /home/gastown 2>/dev/null || true
    log "Ownership fixed"

    # Fix git safe.directory for mounted volume (as root and as gastown)
    log "Configuring git safe.directory..."
    git config --global --add safe.directory /gt || true
    su-exec gastown git config --global --add safe.directory /gt || true

    # Initialize container-local beads DB if not present
    # The container owns its own .beads — never shared with the host
    if [ ! -f "/gt/.beads/beads.db" ]; then
        log "Initializing container-local beads DB..."
        mkdir -p /gt/.beads
        chown -R gastown:gastown /gt/.beads
        su-exec gastown bd init /gt/.beads 2>/dev/null || true
    fi
    if [ ! -f "/home/gastown/.config/beads/config.json" ]; then
        mkdir -p /home/gastown/.config/beads
        chown -R gastown:gastown /home/gastown/.config/beads
        su-exec gastown bd init /gt/.beads 2>/dev/null || true
    fi

    # Initialize tmux server with a holder session
    # Note: tmux start-server alone doesn't persist - we need a session
    log "Initializing tmux server..."
    if ! su-exec gastown tmux has-session -t gt-holder 2>/dev/null; then
        su-exec gastown tmux new-session -d -s gt-holder -x 120 -y 30 "while true; do sleep 3600; done"
        log "Created tmux holder session"
    fi

    # Phase 1: Start daemon first
    log "Phase 1: Starting daemon..."
    su-exec gastown gt daemon start || {
        log "Daemon start returned non-zero, checking status..."
        sleep 2
        if su-exec gastown gt daemon status >/dev/null 2>&1; then
            log "Daemon is running despite start error"
        else
            log "ERROR: Daemon failed to start"
            return 1
        fi
    }
    # Wait for daemon
    local max_wait=60 waited=0
    log "Waiting for daemon (max ${max_wait}s)..."
    while [ $waited -lt $max_wait ]; do
        if su-exec gastown gt daemon status >/dev/null 2>&1; then
            log "Daemon ready after ${waited}s"
            break
        fi
        sleep 1
        ((waited++))
    done
    if [ $waited -ge $max_wait ]; then
        log "ERROR: Daemon not ready after ${max_wait}s"
        return 1
    fi

    # Phase 2: Start deacon
    log "Phase 2: Starting deacon..."
    su-exec gastown gt deacon start || {
        log "Deacon start command completed"
    }
    wait_for_session "hq-deacon" 30

    # Give deacon time to initialize patrol
    log "Allowing deacon patrol initialization..."
    sleep 5

    # Phase 3: Start mayor session
    log "Phase 3: Starting mayor..."
    su-exec gastown gt mayor start || {
        log "Mayor start command completed"
    }
    wait_for_session "hq-mayor" 30

    # Phase 5: Bootstrap execution infrastructure (rig + crew)
    # gt rig add creates its own directory under /gt, which is a bind mount owned by host user.
    # Temporarily make /gt group-writable so gastown can create the rig directory.
    log "Phase 5: Bootstrapping execution infrastructure..."
    if ! su-exec gastown gt rig list --json 2>/dev/null | grep -q '"bench"'; then
        log "No bench rig — adding..."
        # Remove stale directory if exists from previous failed attempt
        rm -rf /gt/bench 2>/dev/null || true
        # Temporarily allow gastown to write to /gt
        chmod g+w /gt
        chgrp gastown /gt
        su-exec gastown gt rig add bench https://github.com/shisuiki/gastown.git --prefix bn || {
            log "WARNING: Failed to add bench rig (non-fatal)"
        }
        # Restore original permissions
        chmod g-w /gt
    else
        log "Bench rig exists, skipping"
    fi

    if ! su-exec gastown gt crew list --rig bench --json 2>/dev/null | jq -e 'length > 0' >/dev/null 2>&1; then
        log "No crew in bench rig — adding worker crew..."
        su-exec gastown gt crew add worker --rig bench || {
            log "WARNING: Failed to add worker crew (non-fatal)"
        }
    else
        log "Crew already configured in bench rig, skipping"
    fi
    log "Execution infrastructure bootstrap complete"

    # Show status
    log "=== Gas Town Status ==="
    su-exec gastown gt status || true

    log "=== Full Gas Town Ready ==="

    # Start web UI in foreground (keeps container alive)
    log "Starting web UI on port 8080..."
    exec su-exec gastown gt gui --port 8080
}

run_gui_only() {
    log "Starting GUI-only mode..."
    exec su-exec gastown gt gui --port 8080
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
