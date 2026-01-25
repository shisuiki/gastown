#!/usr/bin/env bash
#
# extract-tasks.sh - Extract SWE-EVO benchmark tasks
#
# Clones/updates SWE-EVO repository and extracts a balanced subset of tasks
# for benchmark execution.
#
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SWE_EVO_CACHE="${SWE_EVO_CACHE:-/home/shisui/gt/cache/swe-evo}"
SWE_EVO_REPO="${SWE_EVO_REPO:-https://github.com/amazon-science/SWE-EVO.git}"
TASK_COUNT="${TASK_COUNT:-10}"
OUTPUT_DIR="${1:-/home/shisui/gt/logs/swe-evo-tasks}"

log() {
    printf "[%s] %s\n" "$(date -u +%Y-%m-%dT%H:%M:%SZ)" "$*"
}

fail() {
    log "ERROR: $*"
    exit 1
}

# Clone or update SWE-EVO repository
setup_swe_evo() {
    log "Setting up SWE-EVO repository..."

    if [ -d "$SWE_EVO_CACHE/.git" ]; then
        log "Updating existing cache..."
        git -C "$SWE_EVO_CACHE" pull --quiet || log "WARNING: Failed to update cache"
    else
        log "Cloning SWE-EVO repository..."
        mkdir -p "$(dirname "$SWE_EVO_CACHE")"
        git clone --depth 1 "$SWE_EVO_REPO" "$SWE_EVO_CACHE" || fail "Failed to clone SWE-EVO"
    fi

    log "SWE-EVO cache ready at $SWE_EVO_CACHE"
}

# Extract task list from SWE-EVO dataset
# Returns balanced subset across difficulty levels
extract_task_list() {
    local dataset_file="$SWE_EVO_CACHE/data/swe_evo_lite.json"

    if [ ! -f "$dataset_file" ]; then
        # Try alternate locations
        dataset_file="$SWE_EVO_CACHE/swe_evo_lite.json"
        if [ ! -f "$dataset_file" ]; then
            log "Looking for dataset file..."
            dataset_file=$(find "$SWE_EVO_CACHE" -name "*.json" -type f | head -1)
        fi
    fi

    if [ ! -f "$dataset_file" ]; then
        fail "No dataset file found in SWE-EVO cache"
    fi

    log "Using dataset: $dataset_file"

    # Extract balanced subset of tasks
    # Goal: mix of easy, medium, hard tasks from different repos
    python3 - "$dataset_file" "$TASK_COUNT" <<'PYTHON'
import json
import sys
import random

dataset_file = sys.argv[1]
task_count = int(sys.argv[2])

with open(dataset_file, 'r') as f:
    data = json.load(f)

# Handle different dataset formats
if isinstance(data, list):
    tasks = data
elif isinstance(data, dict) and 'instances' in data:
    tasks = data['instances']
else:
    tasks = list(data.values()) if isinstance(data, dict) else []

# Filter to tasks with required fields
valid_tasks = []
for t in tasks:
    if isinstance(t, dict):
        task_id = t.get('instance_id') or t.get('id') or t.get('task_id')
        if task_id:
            valid_tasks.append({
                'instance_id': task_id,
                'repo': t.get('repo', 'unknown'),
                'problem_statement': t.get('problem_statement', t.get('description', '')),
                'base_commit': t.get('base_commit', ''),
                'test_patch': t.get('test_patch', ''),
                'patch': t.get('patch', t.get('gold_patch', '')),
            })

# Sample balanced subset
random.seed(42)  # Reproducible selection
selected = random.sample(valid_tasks, min(task_count, len(valid_tasks)))

# Output as JSON
print(json.dumps(selected, indent=2))
PYTHON
}

# Create task workspace directories
prepare_task_workspaces() {
    local tasks_json="$1"

    mkdir -p "$OUTPUT_DIR"

    log "Preparing task workspaces..."

    echo "$tasks_json" | python3 - "$OUTPUT_DIR" <<'PYTHON'
import json
import sys
import os

tasks = json.load(sys.stdin)
output_dir = sys.argv[1]

manifest = {
    'extracted_at': __import__('datetime').datetime.utcnow().isoformat() + 'Z',
    'task_count': len(tasks),
    'tasks': []
}

for i, task in enumerate(tasks):
    task_id = task['instance_id']
    task_dir = os.path.join(output_dir, task_id.replace('/', '__'))
    os.makedirs(task_dir, exist_ok=True)

    # Write task metadata
    with open(os.path.join(task_dir, 'task.json'), 'w') as f:
        json.dump(task, f, indent=2)

    # Write problem statement for easy reading
    with open(os.path.join(task_dir, 'problem.md'), 'w') as f:
        f.write(f"# {task_id}\n\n")
        f.write(f"**Repo:** {task['repo']}\n\n")
        f.write("## Problem Statement\n\n")
        f.write(task.get('problem_statement', 'No description available'))

    manifest['tasks'].append({
        'instance_id': task_id,
        'repo': task['repo'],
        'workspace': task_dir,
        'status': 'pending'
    })

    print(f"Prepared task {i+1}/{len(tasks)}: {task_id}")

# Write manifest
with open(os.path.join(output_dir, 'manifest.json'), 'w') as f:
    json.dump(manifest, f, indent=2)

print(f"\nManifest written to {os.path.join(output_dir, 'manifest.json')}")
PYTHON
}

main() {
    log "=== SWE-EVO Task Extraction ==="
    log "Task count: $TASK_COUNT"
    log "Output directory: $OUTPUT_DIR"

    # Setup repository
    setup_swe_evo

    # Extract task list
    log "Extracting task list..."
    local tasks_json
    tasks_json=$(extract_task_list)

    if [ -z "$tasks_json" ] || [ "$tasks_json" = "[]" ]; then
        fail "No tasks extracted from dataset"
    fi

    # Prepare workspaces
    prepare_task_workspaces "$tasks_json"

    log "=== Extraction Complete ==="
    log "Tasks ready in: $OUTPUT_DIR"

    # Output manifest location
    echo "$OUTPUT_DIR/manifest.json"
}

main "$@"
