#!/usr/bin/env bash
#
# dispatch-task.sh - Dispatch a SWE-EVO task to canary mayor
#
# Creates a tracking bead, mounts task workspace, and sends BENCHMARK_TASK
# mail to canary mayor for execution.
#
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
CONTAINER_NAME="${CONTAINER_NAME:-gastown-canary}"
TIMEOUT="${TASK_TIMEOUT:-1800}"  # 30 minutes default

log() {
    printf "[%s] %s\n" "$(date -u +%Y-%m-%dT%H:%M:%SZ)" "$*"
}

fail() {
    log "ERROR: $*"
    exit 1
}

usage() {
    echo "Usage: $0 <task-workspace-dir> <benchmark-id>"
    echo ""
    echo "Arguments:"
    echo "  task-workspace-dir  Path to task workspace (contains task.json)"
    echo "  benchmark-id        Benchmark run identifier"
    exit 1
}

[ $# -lt 2 ] && usage

TASK_DIR="$1"
BENCHMARK_ID="$2"

[ ! -d "$TASK_DIR" ] && fail "Task directory not found: $TASK_DIR"
[ ! -f "$TASK_DIR/task.json" ] && fail "task.json not found in $TASK_DIR"

# Extract task metadata
TASK_ID=$(jq -r '.instance_id' "$TASK_DIR/task.json")
REPO=$(jq -r '.repo' "$TASK_DIR/task.json")
PROBLEM_STATEMENT=$(jq -r '.problem_statement' "$TASK_DIR/task.json" | head -c 2000)

log "=== Dispatching Task ==="
log "Task ID: $TASK_ID"
log "Repo: $REPO"
log "Benchmark ID: $BENCHMARK_ID"

# Create tracking bead for this task
log "Creating tracking bead..."
BEAD_ID=$(bd new -t task "$TASK_ID" -p P2 --json 2>/dev/null | jq -r '.id // empty')

if [ -z "$BEAD_ID" ]; then
    # Fallback: generate local tracking ID
    BEAD_ID="swe-evo-$(echo "$TASK_ID" | md5sum | cut -c1-8)"
    log "Using generated bead ID: $BEAD_ID"
else
    log "Created bead: $BEAD_ID"
fi

# Update task status in manifest
MANIFEST_FILE="$(dirname "$TASK_DIR")/manifest.json"
if [ -f "$MANIFEST_FILE" ]; then
    jq --arg tid "$TASK_ID" --arg bid "$BEAD_ID" \
       '(.tasks[] | select(.instance_id == $tid)) |= . + {bead_id: $bid, status: "dispatched", dispatched_at: (now | todate)}' \
       "$MANIFEST_FILE" > "$MANIFEST_FILE.tmp" && mv "$MANIFEST_FILE.tmp" "$MANIFEST_FILE"
fi

# Create task context file for container
CONTEXT_FILE="$TASK_DIR/dispatch-context.json"
cat > "$CONTEXT_FILE" << EOF
{
    "task_id": "$TASK_ID",
    "bead_id": "$BEAD_ID",
    "benchmark_id": "$BENCHMARK_ID",
    "repo": "$REPO",
    "workspace": "/benchmark/task",
    "timeout_seconds": $TIMEOUT,
    "dispatched_at": "$(date -u +%Y-%m-%dT%H:%M:%SZ)",
    "response_address": "nightingale/Nightingale",
    "requirements": {
        "create_sub_beads": true,
        "use_convoy_for_multi_file": true,
        "report_progress_via_mail": true,
        "include_friction_report": true
    }
}
EOF

# Mount task workspace into container
# Note: This requires container restart or volume mounting strategy
log "Preparing container workspace mount..."
CONTAINER_WORKSPACE="/benchmark/$TASK_ID"

# Copy task files to container-accessible location
BENCHMARK_HOST_DIR="/home/shisui/gt/benchmark-workspaces/$BENCHMARK_ID"
mkdir -p "$BENCHMARK_HOST_DIR/$TASK_ID"
cp -r "$TASK_DIR"/* "$BENCHMARK_HOST_DIR/$TASK_ID/"

log "Task files staged at: $BENCHMARK_HOST_DIR/$TASK_ID"

# Send BENCHMARK_TASK mail to canary mayor
log "Sending BENCHMARK_TASK mail..."

MAIL_BODY="## BENCHMARK_TASK: $TASK_ID

**Benchmark ID:** $BENCHMARK_ID
**Bead ID:** $BEAD_ID
**Timeout:** ${TIMEOUT}s

### Problem Statement

$PROBLEM_STATEMENT

### Workspace

Task files at: /gt/benchmark-workspaces/$BENCHMARK_ID/$TASK_ID

### Requirements

You MUST:
1. Create sub-beads for investigation and implementation
2. Use convoy for multi-file changes
3. Report progress via mail to nightingale/Nightingale
4. Include FRICTION_REPORT section in completion mail

### Response Protocol

When complete, send mail with subject: BENCHMARK_COMPLETE: $TASK_ID

Include in body:
- STATUS: SUCCESS | PARTIAL | FAILED
- CHANGES: List of files modified
- FRICTION_REPORT: Any workflow frictions encountered
- TIME_SPENT: Approximate time on task

### Timeout

This task will timeout in ${TIMEOUT} seconds. Partial progress is acceptable."

sudo docker exec "$CONTAINER_NAME" bash -c "export BEADS_DIR=/gt/.beads && gt mail send mayor/ -s 'BENCHMARK_TASK: $TASK_ID' -m '$MAIL_BODY'" 2>&1 || {
    log "WARNING: Failed to send mail via container, trying host fallback"
    gt mail send mayor/ -s "BENCHMARK_TASK: $TASK_ID" -m "$MAIL_BODY" 2>&1 || fail "Failed to send benchmark task mail"
}

log "Mail sent to canary mayor"

# Nudge mayor to process
log "Nudging mayor..."
sudo docker exec "$CONTAINER_NAME" bash -c "export BEADS_DIR=/gt/.beads && gt nudge mayor 'BENCHMARK_TASK received: $TASK_ID. Check mail and begin work.'" 2>&1 || true

# Record dispatch in task directory
cat > "$TASK_DIR/dispatch-status.json" << EOF
{
    "status": "dispatched",
    "dispatched_at": "$(date -u +%Y-%m-%dT%H:%M:%SZ)",
    "bead_id": "$BEAD_ID",
    "benchmark_id": "$BENCHMARK_ID",
    "timeout_seconds": $TIMEOUT,
    "container": "$CONTAINER_NAME"
}
EOF

log "=== Dispatch Complete ==="
log "Task $TASK_ID dispatched to canary mayor"
log "Waiting for BENCHMARK_COMPLETE or timeout (${TIMEOUT}s)"

# Output dispatch info
echo "$TASK_DIR/dispatch-status.json"
