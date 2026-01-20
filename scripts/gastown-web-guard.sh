#!/bin/bash
# gastown-web-guard.sh - Prevent port conflicts for system-level web service.

set -euo pipefail

PORT="${GASTOWN_WEB_PORT:-8080}"

extract_pids() {
    ss -lptn "sport = :${PORT}" 2>/dev/null \
        | awk -F'pid=' 'NR>1 {for (i=2; i<=NF; i++) {split($i,a,","); print a[1]}}' \
        | sort -u
}

pids="$(extract_pids || true)"
if [ -z "$pids" ]; then
    exit 0
fi

for pid in $pids; do
    if [ ! -r "/proc/${pid}/cmdline" ]; then
        continue
    fi
    cmd="$(tr '\0' ' ' < "/proc/${pid}/cmdline")"
    if [[ "$cmd" == *"gt"* && "$cmd" == *"gui"* ]]; then
        echo "[guard] Found gt gui on port ${PORT} (pid ${pid}), stopping..."
        kill "${pid}" 2>/dev/null || true
        sleep 1
        if kill -0 "${pid}" 2>/dev/null; then
            echo "[guard] Process ${pid} still alive; sending SIGKILL."
            kill -9 "${pid}" 2>/dev/null || true
        fi
    else
        echo "[guard] Port ${PORT} in use by another process: ${cmd}"
        exit 1
    fi
done
