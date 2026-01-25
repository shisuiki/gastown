#!/usr/bin/env bash
#
# aggregate-results.sh - Aggregate SWE-EVO benchmark results
#
# Compiles all task results into a final report with:
# - Overall Resolved Rate and Fix Rate
# - Per-task breakdown
# - Friction report aggregation
# - Auto-creation of improvement beads
#
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPORTS_DIR="${REPORTS_DIR:-/home/shisui/gt/nightingale/cicd/reports/swe-evo}"

log() {
    printf "[%s] %s\n" "$(date -u +%Y-%m-%dT%H:%M:%SZ)" "$*"
}

fail() {
    log "ERROR: $*"
    exit 1
}

usage() {
    echo "Usage: $0 <benchmark-id> <tasks-dir>"
    echo ""
    echo "Arguments:"
    echo "  benchmark-id  Benchmark run identifier"
    echo "  tasks-dir     Directory containing task results"
    exit 1
}

[ $# -lt 2 ] && usage

BENCHMARK_ID="$1"
TASKS_DIR="$2"

[ ! -d "$TASKS_DIR" ] && fail "Tasks directory not found: $TASKS_DIR"

log "=== Aggregating Benchmark Results ==="
log "Benchmark ID: $BENCHMARK_ID"
log "Tasks directory: $TASKS_DIR"

# Create benchmark report directory
BENCHMARK_REPORT_DIR="$REPORTS_DIR/$BENCHMARK_ID"
mkdir -p "$BENCHMARK_REPORT_DIR/tasks"

# Aggregate task results
aggregate_task_results() {
    local total=0
    local resolved_count=0
    local total_fix_rate=0
    local completed_count=0
    local timeout_count=0
    local failed_count=0

    local all_frictions=""

    # Process each task directory
    for task_dir in "$TASKS_DIR"/*/; do
        [ ! -d "$task_dir" ] && continue

        local task_name=$(basename "$task_dir")
        local grade_file="$task_dir/grade-result.json"

        ((total++)) || true

        if [ -f "$grade_file" ]; then
            # Copy grade result to report
            cp "$grade_file" "$BENCHMARK_REPORT_DIR/tasks/${task_name}.json"

            local status=$(jq -r '.status' "$grade_file")
            local resolved=$(jq -r '.resolved' "$grade_file")
            local fix_rate=$(jq -r '.fix_rate' "$grade_file")
            local friction=$(jq -r '.friction_report' "$grade_file")

            case "$status" in
                "SUCCESS"|"PARTIAL")
                    ((completed_count++)) || true
                    if [ "$resolved" = "true" ]; then
                        ((resolved_count++)) || true
                    fi
                    total_fix_rate=$(echo "$total_fix_rate + $fix_rate" | bc)
                    ;;
                "timeout")
                    ((timeout_count++)) || true
                    ;;
                *)
                    ((failed_count++)) || true
                    ;;
            esac

            # Collect friction reports
            if [ -n "$friction" ] && [ "$friction" != "null" ] && [ "$friction" != "N/A" ]; then
                all_frictions="${all_frictions}### $task_name\n$friction\n\n"
            fi
        else
            log "WARNING: No grade result for $task_name"
            ((failed_count++)) || true
        fi
    done

    # Calculate aggregate metrics
    local resolved_rate=0
    local avg_fix_rate=0

    if [ "$total" -gt 0 ]; then
        resolved_rate=$(echo "scale=4; $resolved_count / $total" | bc)
    fi

    if [ "$completed_count" -gt 0 ]; then
        avg_fix_rate=$(echo "scale=4; $total_fix_rate / $completed_count" | bc)
    fi

    # Output metrics
    echo "$total $resolved_count $resolved_rate $avg_fix_rate $completed_count $timeout_count $failed_count"

    # Write frictions to file
    echo -e "$all_frictions" > "$BENCHMARK_REPORT_DIR/all-frictions.md"
}

# Parse friction reports and rank by frequency
analyze_frictions() {
    local frictions_file="$BENCHMARK_REPORT_DIR/all-frictions.md"

    if [ ! -s "$frictions_file" ]; then
        log "No friction reports to analyze"
        return
    fi

    log "Analyzing friction reports..."

    # Extract and count friction categories
    python3 - "$frictions_file" "$BENCHMARK_REPORT_DIR/friction-summary.md" <<'PYTHON'
import sys
import re
from collections import Counter

frictions_file = sys.argv[1]
output_file = sys.argv[2]

with open(frictions_file, 'r') as f:
    content = f.read()

# Extract friction points (look for common patterns)
friction_patterns = [
    (r'timeout', 'Timeout issues'),
    (r'permission', 'Permission issues'),
    (r'not found|missing', 'Missing resources'),
    (r'slow|performance', 'Performance issues'),
    (r'confus|unclear', 'Unclear documentation'),
    (r'error|fail', 'Errors/Failures'),
    (r'mail|message', 'Mail system issues'),
    (r'bead|issue', 'Beads system issues'),
    (r'git|commit', 'Git workflow issues'),
    (r'docker|container', 'Container issues'),
]

categories = Counter()
details = {}

for pattern, category in friction_patterns:
    matches = re.findall(rf'.*{pattern}.*', content, re.IGNORECASE)
    if matches:
        categories[category] = len(matches)
        details[category] = matches[:3]  # Keep top 3 examples

# Write summary
with open(output_file, 'w') as f:
    f.write("# Friction Summary\n\n")
    f.write(f"**Benchmark:** {sys.argv[1].split('/')[-2]}\n\n")

    if categories:
        f.write("## Top Friction Categories\n\n")
        f.write("| Category | Occurrences | Priority |\n")
        f.write("|----------|-------------|----------|\n")

        for i, (category, count) in enumerate(categories.most_common(10)):
            priority = "P1" if i < 3 else "P2" if i < 6 else "P3"
            f.write(f"| {category} | {count} | {priority} |\n")

        f.write("\n## Details\n\n")
        for category, examples in details.items():
            f.write(f"### {category}\n\n")
            for ex in examples:
                f.write(f"- {ex.strip()[:100]}...\n")
            f.write("\n")
    else:
        f.write("No significant friction patterns detected.\n")

print(f"Friction summary written to {output_file}")
PYTHON
}

# Auto-create improvement beads for top friction points
create_improvement_beads() {
    local summary_file="$BENCHMARK_REPORT_DIR/friction-summary.md"

    if [ ! -f "$summary_file" ]; then
        log "No friction summary to process"
        return
    fi

    log "Creating improvement beads for top friction points..."

    # Extract top 3 friction categories
    local top_frictions
    top_frictions=$(grep -E "^\| [^|]+ \| [0-9]+ \| P1 \|$" "$summary_file" | head -3 || true)

    if [ -z "$top_frictions" ]; then
        log "No P1 friction points found"
        return
    fi

    echo "$top_frictions" | while read -r line; do
        local category=$(echo "$line" | cut -d'|' -f2 | xargs)
        local count=$(echo "$line" | cut -d'|' -f3 | xargs)

        if [ -n "$category" ]; then
            log "Creating bead for: $category ($count occurrences)"

            bd new -t task \
                "SWE-EVO Friction: $category" \
                -d "Auto-generated from benchmark $BENCHMARK_ID.

Friction category: $category
Occurrences: $count

See: $summary_file for details.

This improvement task was automatically created from the SWE-EVO benchmark friction pipeline." \
                -l friction,auto-generated,swe-evo \
                -p P2 2>/dev/null || log "WARNING: Failed to create bead for $category"
        fi
    done
}

# Main aggregation
read -r TOTAL RESOLVED RR AFR COMPLETED TIMEOUT FAILED <<< "$(aggregate_task_results)"

log "Tasks: $TOTAL total, $COMPLETED completed, $TIMEOUT timeout, $FAILED failed"
log "Resolved: $RESOLVED ($RR)"
log "Avg Fix Rate: $AFR"

# Write final report
cat > "$BENCHMARK_REPORT_DIR/report.json" << EOF
{
    "benchmark_id": "$BENCHMARK_ID",
    "completed_at": "$(date -u +%Y-%m-%dT%H:%M:%SZ)",
    "tasks": {
        "total": $TOTAL,
        "completed": $COMPLETED,
        "timeout": $TIMEOUT,
        "failed": $FAILED
    },
    "metrics": {
        "resolved_count": $RESOLVED,
        "resolved_rate": $RR,
        "average_fix_rate": $AFR
    },
    "reports": {
        "tasks_dir": "$BENCHMARK_REPORT_DIR/tasks",
        "friction_summary": "$BENCHMARK_REPORT_DIR/friction-summary.md",
        "all_frictions": "$BENCHMARK_REPORT_DIR/all-frictions.md"
    }
}
EOF

# Analyze frictions
analyze_frictions

# Create improvement beads
create_improvement_beads

# Update latest symlink
ln -sf "$BENCHMARK_ID" "$REPORTS_DIR/latest"

log "=== Aggregation Complete ==="
log "Report: $BENCHMARK_REPORT_DIR/report.json"

cat "$BENCHMARK_REPORT_DIR/report.json"
