#!/bin/bash
# gastown-sync.sh - Auto-sync gastown-src and rebuild gt
#
# Usage:
#   ./gastown-sync.sh check    - Check if behind remote
#   ./gastown-sync.sh sync     - Pull and rebuild if behind
#   ./gastown-sync.sh watch    - Run as daemon, check every N seconds
#   ./gastown-sync.sh webhook  - Handle webhook trigger (for GitHub Actions)

set -e

SRC_DIR="${GASTOWN_SRC:-$HOME/laplace/gastown-src}"
GT_BIN="$HOME/go/bin/gt"
LOG_FILE="${GASTOWN_SYNC_LOG:-$HOME/gt/logs/gastown-sync.log}"
PID_FILE="$HOME/gt/.gastown-sync.pid"
CHECK_INTERVAL="${GASTOWN_SYNC_INTERVAL:-60}"  # seconds

mkdir -p "$(dirname "$LOG_FILE")"

log() {
    echo "[$(date '+%Y-%m-%d %H:%M:%S')] $*" | tee -a "$LOG_FILE"
}

check_behind() {
    cd "$SRC_DIR"
    git fetch origin main --quiet 2>/dev/null

    LOCAL=$(git rev-parse HEAD)
    REMOTE=$(git rev-parse origin/main)

    if [ "$LOCAL" != "$REMOTE" ]; then
        BEHIND=$(git rev-list --count HEAD..origin/main)
        echo "$BEHIND"
        return 0
    else
        echo "0"
        return 0
    fi
}

do_sync() {
    cd "$SRC_DIR"

    BEHIND=$(check_behind)
    if [ "$BEHIND" = "0" ]; then
        log "Already up to date"
        return 0
    fi

    log "Behind by $BEHIND commit(s), pulling..."

    # Pull (this triggers post-merge hook which builds gt)
    if git pull origin main 2>&1 | tee -a "$LOG_FILE"; then
        log "Pull successful"

        # Verify build happened (post-merge should handle this)
        NEW_COMMIT=$(git rev-parse --short HEAD)
        log "Now at commit: $NEW_COMMIT"

        # Restart daemon if running
        if "$GT_BIN" daemon status 2>/dev/null | grep -q "running"; then
            log "Restarting gt daemon..."
            "$GT_BIN" daemon stop 2>&1 | tee -a "$LOG_FILE" || true
            sleep 2
            "$GT_BIN" daemon start 2>&1 | tee -a "$LOG_FILE"
            log "Daemon restarted"
        fi

        # Restart web service if managed by systemd (user scope)
        if systemctl --user is-active gastown-web.service &>/dev/null; then
            log "Restarting gastown-web.service..."
            systemctl --user restart gastown-web.service
            log "Web service restarted"
        elif systemctl is-active gastown-gui.service &>/dev/null; then
            log "WARNING: system-level gastown-gui.service is active and owns port 8080."
            log "Disable it or migrate to user-level gastown-web.service to enable auto-redeploy."
        fi

        return 0
    else
        log "ERROR: Pull failed"
        return 1
    fi
}

watch_loop() {
    log "Starting watch daemon (interval: ${CHECK_INTERVAL}s)"
    echo $$ > "$PID_FILE"

    trap "rm -f '$PID_FILE'; log 'Watch daemon stopped'; exit 0" SIGTERM SIGINT

    while true; do
        BEHIND=$(check_behind 2>/dev/null || echo "error")

        if [ "$BEHIND" = "error" ]; then
            log "Error checking remote, will retry..."
        elif [ "$BEHIND" != "0" ]; then
            log "Detected $BEHIND new commit(s)"
            do_sync || log "Sync failed, will retry next interval"
        fi

        sleep "$CHECK_INTERVAL"
    done
}

webhook_handler() {
    # For GitHub Actions webhook - immediately sync
    log "Webhook triggered"
    do_sync
}

case "${1:-check}" in
    check)
        BEHIND=$(check_behind)
        if [ "$BEHIND" = "0" ]; then
            echo "Up to date"
        else
            echo "Behind by $BEHIND commit(s)"
        fi
        ;;
    sync)
        do_sync
        ;;
    watch)
        watch_loop
        ;;
    webhook)
        webhook_handler
        ;;
    stop)
        if [ -f "$PID_FILE" ]; then
            kill "$(cat "$PID_FILE")" 2>/dev/null && echo "Watch daemon stopped"
            rm -f "$PID_FILE"
        else
            echo "Watch daemon not running"
        fi
        ;;
    status)
        if [ -f "$PID_FILE" ] && kill -0 "$(cat "$PID_FILE")" 2>/dev/null; then
            echo "Watch daemon running (PID: $(cat "$PID_FILE"))"
        else
            echo "Watch daemon not running"
        fi
        ;;
    *)
        echo "Usage: $0 {check|sync|watch|webhook|stop|status}"
        exit 1
        ;;
esac
