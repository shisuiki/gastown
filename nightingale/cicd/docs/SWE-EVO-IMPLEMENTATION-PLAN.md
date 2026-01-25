# SWE-EVO Benchmark Implementation Plan

## Overview

SWE-EVO benchmark integration for canary-gastown mayor with dual goals:
1. **Benchmarking**: Measure mayor's code evolution capabilities
2. **Recursive Self-Improvement**: Collect friction reports for infrastructure enhancement

## Architecture

```
nightingale/cicd/
├── scripts/
│   └── swe-evo/
│       ├── extract-tasks.sh      # Clone SWE-EVO, extract subset
│       ├── dispatch-task.sh      # Mount workspace, send mail
│       ├── grade-task.sh         # Run tests, calculate metrics
│       ├── aggregate-results.sh  # Compile report, friction pipeline
│       └── generate-summary.sh   # Human-readable output
├── docs/
│   └── SWE-EVO-IMPLEMENTATION-PLAN.md  # This file
└── reports/
    └── swe-evo/
        ├── latest.json           # Current benchmark results
        └── $BENCHMARK_ID/
            ├── report.json
            ├── friction-summary.md
            └── tasks/
```

## Key Metrics

### Resolved Rate (RR)
- Percentage of tasks where all tests pass
- Formula: `resolved_tasks / total_tasks`

### Fix Rate (FR)
- Percentage of previously failing tests now passing
- Formula: `(baseline_failing - still_failing) / baseline_failing`

## Task Selection

10 tasks from SWE-EVO balanced across:
- Difficulty levels (easy, medium, hard)
- Repository types (Django, Flask, etc.)
- Problem categories (bug fix, feature, refactor)

## Execution Flow

1. **Extract Tasks** (`extract-tasks.sh`)
   - Clone/update SWE-EVO repository
   - Select balanced subset
   - Create task workspaces with metadata

2. **Dispatch Tasks** (`dispatch-task.sh`)
   - Create tracking bead
   - Stage workspace in container-accessible location
   - Send BENCHMARK_TASK mail to mayor
   - Nudge mayor to begin

3. **Monitor Execution**
   - Poll inbox for BENCHMARK_COMPLETE
   - Enforce 30-minute timeout per task
   - Collect completion status and friction reports

4. **Grade Results** (`grade-task.sh`)
   - Run pytest in container
   - Parse test output
   - Calculate RR and FR

5. **Aggregate Results** (`aggregate-results.sh`)
   - Compile all task results
   - Analyze friction patterns
   - Auto-create improvement beads

6. **Report** (`generate-summary.sh`)
   - Human-readable summary
   - Mail to mayor
   - Update latest.json

## Friction Pipeline

### Collection
Mayor includes FRICTION_REPORT in completion mail:
```
FRICTION_REPORT:
- Slow mail delivery caused timeout
- Beads command unclear for sub-issue creation
- Git push required sudo
```

### Analysis
- Extract patterns using keyword matching
- Categorize: timeout, permission, missing, unclear, error
- Rank by frequency

### Action
- Create P2 beads for top 3 friction points
- Labels: friction, auto-generated, swe-evo
- Feed into recursive improvement cycle

## Mail Protocol

### BENCHMARK_TASK (Nightingale → Mayor)
```
Subject: BENCHMARK_TASK: <task-id>

## Problem Statement
<issue description>

## Workspace
/gt/benchmark-workspaces/<benchmark-id>/<task-id>

## Requirements
- Create sub-beads for investigation/implementation
- Use convoy for multi-file changes
- Include FRICTION_REPORT in completion
```

### BENCHMARK_COMPLETE (Mayor → Nightingale)
```
Subject: BENCHMARK_COMPLETE: <task-id>

STATUS: SUCCESS | PARTIAL | FAILED
CHANGES: <list of modified files>
FRICTION_REPORT: <workflow frictions>
TIME_SPENT: <approximate duration>
```

## Timeouts

| Stage | Timeout |
|-------|---------|
| Task extraction | 5 min |
| Task dispatch | 1 min |
| Task execution | 30 min |
| Test grading | 5 min |
| Total benchmark | 6 hours |

## Success Criteria

1. All 10 tasks execute with timeout enforcement
2. RR/FR metrics calculated correctly
3. Friction reports collected from all completed tasks
4. At least 1 improvement bead auto-created
5. Report visible in CI/CD panel via latest.json

## Formula

Molecule formula: `.beads/formulas/mol-swe-evo-benchmark.formula.toml`

Execute with:
```bash
bd mol wisp mol-swe-evo-benchmark
```

## Future Enhancements

- Parallel task execution
- Adaptive timeout based on task complexity
- Integration with external CI/CD systems
- Comparison across mayor versions
- Automated re-run on infrastructure changes
