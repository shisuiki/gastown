#!/usr/bin/env bash
#
# grade-task.sh - Grade a SWE-EVO task execution
#
# Runs tests in the canary container and calculates:
# - Resolved Rate (RR): All tests pass
# - Fix Rate (FR): Previously failing tests now pass
#
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
CONTAINER_NAME="${CONTAINER_NAME:-gastown-canary}"
TEST_TIMEOUT="${TEST_TIMEOUT:-300}"  # 5 minutes for test execution

log() {
    printf "[%s] %s\n" "$(date -u +%Y-%m-%dT%H:%M:%SZ)" "$*"
}

fail() {
    log "ERROR: $*"
    exit 1
}

usage() {
    echo "Usage: $0 <task-workspace-dir>"
    echo ""
    echo "Arguments:"
    echo "  task-workspace-dir  Path to task workspace (contains task.json)"
    exit 1
}

[ $# -lt 1 ] && usage

TASK_DIR="$1"
[ ! -d "$TASK_DIR" ] && fail "Task directory not found: $TASK_DIR"
[ ! -f "$TASK_DIR/task.json" ] && fail "task.json not found in $TASK_DIR"

# Extract task metadata
TASK_ID=$(jq -r '.instance_id' "$TASK_DIR/task.json")
REPO=$(jq -r '.repo' "$TASK_DIR/task.json")
TEST_PATCH=$(jq -r '.test_patch // empty' "$TASK_DIR/task.json")

log "=== Grading Task ==="
log "Task ID: $TASK_ID"
log "Repo: $REPO"

GRADE_RESULT="$TASK_DIR/grade-result.json"

# Check for completion status
if [ ! -f "$TASK_DIR/completion-status.json" ]; then
    log "No completion status found - task may have timed out"

    cat > "$GRADE_RESULT" << EOF
{
    "task_id": "$TASK_ID",
    "graded_at": "$(date -u +%Y-%m-%dT%H:%M:%SZ)",
    "status": "timeout",
    "resolved": false,
    "fix_rate": 0,
    "tests_passed": 0,
    "tests_failed": 0,
    "tests_total": 0,
    "error": "Task did not complete within timeout"
}
EOF
    cat "$GRADE_RESULT"
    exit 0
fi

COMPLETION_STATUS=$(jq -r '.status' "$TASK_DIR/completion-status.json")
log "Completion status: $COMPLETION_STATUS"

# Determine test command based on repo type
determine_test_command() {
    local repo="$1"

    # Common Python test frameworks
    if [[ "$repo" == *"django"* ]]; then
        echo "python -m pytest --tb=short -q"
    elif [[ "$repo" == *"flask"* ]]; then
        echo "python -m pytest --tb=short -q"
    elif [[ "$repo" == *"requests"* ]]; then
        echo "python -m pytest --tb=short -q"
    else
        # Default to pytest
        echo "python -m pytest --tb=short -q"
    fi
}

# Run tests in container
run_tests() {
    local workspace="$1"
    local test_cmd="$2"

    log "Running tests: $test_cmd"

    local test_output
    local exit_code

    # Run tests with timeout
    set +e
    test_output=$(timeout "$TEST_TIMEOUT" sudo docker exec "$CONTAINER_NAME" \
        bash -c "cd $workspace && $test_cmd 2>&1" 2>&1)
    exit_code=$?
    set -e

    echo "$test_output"
    return $exit_code
}

# Parse pytest output for pass/fail counts
parse_test_results() {
    local output="$1"

    # Try to extract from pytest summary line
    # Format: "X passed, Y failed, Z errors in N.NNs"
    local passed=0
    local failed=0
    local errors=0

    if echo "$output" | grep -qE "[0-9]+ passed"; then
        passed=$(echo "$output" | grep -oE "[0-9]+ passed" | grep -oE "[0-9]+")
    fi

    if echo "$output" | grep -qE "[0-9]+ failed"; then
        failed=$(echo "$output" | grep -oE "[0-9]+ failed" | grep -oE "[0-9]+")
    fi

    if echo "$output" | grep -qE "[0-9]+ error"; then
        errors=$(echo "$output" | grep -oE "[0-9]+ error" | grep -oE "[0-9]+")
    fi

    echo "$passed $failed $errors"
}

# Calculate metrics
calculate_metrics() {
    local passed=$1
    local failed=$2
    local errors=$3
    local baseline_failed=${4:-0}

    local total=$((passed + failed + errors))
    local resolved=false
    local fix_rate=0

    # Resolved Rate: All tests pass (no failures, no errors)
    if [ "$failed" -eq 0 ] && [ "$errors" -eq 0 ] && [ "$total" -gt 0 ]; then
        resolved=true
    fi

    # Fix Rate: Proportion of previously failing tests now passing
    if [ "$baseline_failed" -gt 0 ]; then
        local newly_passing=$((baseline_failed - failed))
        if [ "$newly_passing" -lt 0 ]; then
            newly_passing=0
        fi
        fix_rate=$(echo "scale=2; $newly_passing / $baseline_failed" | bc)
    fi

    echo "$resolved $fix_rate"
}

# Main grading logic
BENCHMARK_ID=$(jq -r '.benchmark_id // "unknown"' "$TASK_DIR/dispatch-status.json" 2>/dev/null || echo "unknown")
WORKSPACE="/gt/benchmark-workspaces/$BENCHMARK_ID/$TASK_ID"

# Get test command
TEST_CMD=$(determine_test_command "$REPO")

# Run tests
log "Executing tests in container..."
set +e
TEST_OUTPUT=$(run_tests "$WORKSPACE" "$TEST_CMD")
TEST_EXIT=$?
set -e

log "Test exit code: $TEST_EXIT"

# Parse results
read -r PASSED FAILED ERRORS <<< "$(parse_test_results "$TEST_OUTPUT")"
PASSED=${PASSED:-0}
FAILED=${FAILED:-0}
ERRORS=${ERRORS:-0}

log "Results: $PASSED passed, $FAILED failed, $ERRORS errors"

# Get baseline (from test_patch metadata if available)
BASELINE_FAILED=0
if [ -n "$TEST_PATCH" ] && [ "$TEST_PATCH" != "null" ]; then
    # Estimate baseline from test patch (simplified)
    BASELINE_FAILED=$(echo "$TEST_PATCH" | grep -c "def test_" || echo "0")
fi

# Calculate metrics
read -r RESOLVED FIX_RATE <<< "$(calculate_metrics "$PASSED" "$FAILED" "$ERRORS" "$BASELINE_FAILED")"

# Get friction report from completion status
FRICTION_REPORT=$(jq -r '.friction_report // "No friction report provided"' "$TASK_DIR/completion-status.json" 2>/dev/null || echo "N/A")

# Write grade result
cat > "$GRADE_RESULT" << EOF
{
    "task_id": "$TASK_ID",
    "graded_at": "$(date -u +%Y-%m-%dT%H:%M:%SZ)",
    "status": "$COMPLETION_STATUS",
    "resolved": $RESOLVED,
    "fix_rate": $FIX_RATE,
    "tests_passed": $PASSED,
    "tests_failed": $FAILED,
    "tests_errors": $ERRORS,
    "tests_total": $((PASSED + FAILED + ERRORS)),
    "test_exit_code": $TEST_EXIT,
    "friction_report": $(echo "$FRICTION_REPORT" | jq -Rs .),
    "test_output_excerpt": $(echo "$TEST_OUTPUT" | tail -20 | jq -Rs .)
}
EOF

log "=== Grading Complete ==="
log "Resolved: $RESOLVED"
log "Fix Rate: $FIX_RATE"

cat "$GRADE_RESULT"
