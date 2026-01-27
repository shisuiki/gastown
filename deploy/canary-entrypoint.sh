#!/bin/bash
#
# canary-entrypoint.sh - Canary container entrypoint
#
# Sets up environment and starts gt gui or other commands.
#

set -euo pipefail

log() {
    echo "[$(date -u +%Y-%m-%dT%H:%M:%SZ)] [entrypoint] $*"
}

setup_git() {
    # Configure git safe.directory for mounted volumes
    git config --global --add safe.directory /home/gastown/gt 2>/dev/null || true
    git config --global --add safe.directory /home/gastown/gt-live 2>/dev/null || true
}

setup_claude() {
    # Claude credentials are mounted from host at runtime
    # Check if credentials exist
    if [ -f "/home/gastown/.claude/.credentials.json" ]; then
        log "Claude credentials found"
    else
        log "WARNING: Claude credentials not mounted - claude will require login"
    fi
}

setup_beads() {
    # Initialize beads if .beads directory exists
    if [ -d "/home/gastown/gt/.beads" ]; then
        log "Initializing beads..."
        bd init /home/gastown/gt/.beads 2>/dev/null || true
    fi
}

main() {
    log "=== Canary Container Starting ==="
    log "HOME: $HOME"
    log "GT_ROOT: ${GT_ROOT:-/home/gastown/gt}"
    log "Working directory: $(pwd)"

    setup_git
    setup_claude
    setup_beads

    log "=== Setup Complete ==="

    # Execute the command
    if [ "$1" = "gui" ]; then
        log "Starting gt gui..."
        exec gt "$@"
    elif [ "$1" = "shell" ]; then
        log "Starting interactive shell..."
        exec /bin/bash
    else
        log "Executing: $*"
        exec "$@"
    fi
}

main "$@"
