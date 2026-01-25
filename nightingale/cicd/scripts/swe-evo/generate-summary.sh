#!/usr/bin/env bash
#
# generate-summary.sh - Generate human-readable SWE-EVO benchmark summary
#
set -euo pipefail

REPORTS_DIR="${REPORTS_DIR:-/home/shisui/gt/nightingale/cicd/reports/swe-evo}"

usage() {
    echo "Usage: $0 [benchmark-id]"
    echo ""
    echo "Arguments:"
    echo "  benchmark-id  Benchmark to summarize (default: latest)"
    exit 1
}

BENCHMARK_ID="${1:-latest}"

if [ "$BENCHMARK_ID" = "latest" ]; then
    if [ -L "$REPORTS_DIR/latest" ]; then
        BENCHMARK_ID=$(readlink "$REPORTS_DIR/latest")
    else
        echo "No latest benchmark found"
        exit 1
    fi
fi

REPORT_DIR="$REPORTS_DIR/$BENCHMARK_ID"
REPORT_FILE="$REPORT_DIR/report.json"

[ ! -f "$REPORT_FILE" ] && { echo "Report not found: $REPORT_FILE"; exit 1; }

# Extract metrics
TOTAL=$(jq -r '.tasks.total' "$REPORT_FILE")
COMPLETED=$(jq -r '.tasks.completed' "$REPORT_FILE")
TIMEOUT=$(jq -r '.tasks.timeout' "$REPORT_FILE")
FAILED=$(jq -r '.tasks.failed' "$REPORT_FILE")
RESOLVED=$(jq -r '.metrics.resolved_count' "$REPORT_FILE")
RR=$(jq -r '.metrics.resolved_rate' "$REPORT_FILE")
AFR=$(jq -r '.metrics.average_fix_rate' "$REPORT_FILE")
COMPLETED_AT=$(jq -r '.completed_at' "$REPORT_FILE")

# Calculate percentages
RR_PCT=$(echo "scale=1; $RR * 100" | bc)
AFR_PCT=$(echo "scale=1; $AFR * 100" | bc)

cat << EOF
╔══════════════════════════════════════════════════════════════════╗
║                    SWE-EVO Benchmark Summary                      ║
╠══════════════════════════════════════════════════════════════════╣
║  Benchmark ID: $BENCHMARK_ID
║  Completed: $COMPLETED_AT
╠══════════════════════════════════════════════════════════════════╣
║  TASK EXECUTION                                                   ║
║  ───────────────                                                  ║
║  Total Tasks:    $TOTAL
║  Completed:      $COMPLETED
║  Timeout:        $TIMEOUT
║  Failed:         $FAILED
╠══════════════════════════════════════════════════════════════════╣
║  METRICS                                                          ║
║  ───────                                                          ║
║  Resolved Rate:      ${RR_PCT}% ($RESOLVED/$TOTAL tasks fully resolved)
║  Average Fix Rate:   ${AFR_PCT}%
╠══════════════════════════════════════════════════════════════════╣
║  REPORTS                                                          ║
║  ───────                                                          ║
║  Full Report:     $REPORT_FILE
║  Friction Summary: $REPORT_DIR/friction-summary.md
╚══════════════════════════════════════════════════════════════════╝

EOF

# Show friction summary if available
if [ -f "$REPORT_DIR/friction-summary.md" ]; then
    echo "Top Friction Points:"
    echo "───────────────────"
    grep -E "^\| [^|]+ \| [0-9]+ \| P[123] \|$" "$REPORT_DIR/friction-summary.md" | head -5 || echo "None detected"
    echo ""
fi

# Show per-task results
echo "Per-Task Results:"
echo "─────────────────"

for task_file in "$REPORT_DIR/tasks"/*.json; do
    [ ! -f "$task_file" ] && continue

    task_name=$(basename "$task_file" .json)
    status=$(jq -r '.status' "$task_file")
    resolved=$(jq -r '.resolved' "$task_file")
    passed=$(jq -r '.tests_passed' "$task_file")
    failed=$(jq -r '.tests_failed' "$task_file")

    resolved_mark="✗"
    [ "$resolved" = "true" ] && resolved_mark="✓"

    printf "  %s %-40s %s (passed: %d, failed: %d)\n" "$resolved_mark" "$task_name" "$status" "$passed" "$failed"
done
