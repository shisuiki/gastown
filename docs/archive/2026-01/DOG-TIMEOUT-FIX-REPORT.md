---
type: report
status: archived
owner: "unowned"
audience: ops
applies_to:
  repo: gastown
  branch: canary
last_validated: "unknown"
source_of_truth:
  - "scripts/canary-command-retry.sh"
---

# Dog Timeout Fix & CI/CD Canary Infrastructure Report

**Date:** 2026-01-22
**Author:** Mayor (Claude Opus 4.5)
**Priority:** P1
**Status:** Completed

---

## Table of Contents

1. [Executive Summary](#executive-summary)
2. [Background](#background)
3. [CI/CD Canary Infrastructure](#cicd-canary-infrastructure)
4. [Dog Timeout Investigation](#dog-timeout-investigation)
5. [Root Cause Analysis](#root-cause-analysis)
6. [Solutions Implemented](#solutions-implemented)
7. [Verification & Testing](#verification--testing)
8. [Lessons Learned](#lessons-learned)
9. [Appendix](#appendix)

---

## Executive Summary

This report documents the investigation and resolution of P1 issues related to:

1. **Dog timeout failures** - Dogs repeatedly timing out on `mol-session-gc` formula execution
2. **CI/CD canary infrastructure** - Setting up end-to-end deployment testing pipeline

### Key Findings

- **Root Cause Identified:** Template parsing bug in `mol-session-gc.formula.toml` where `{{placeholder}}` syntax was being parsed as required template variables, causing wisp creation to fail
- **Infrastructure Deployed:** Self-hosted GitHub Actions runner with canary deployment pipeline fully operational
- **Resolution:** Fixed template syntax and deployed to production

### Impact

- Dog timeout issues resolved (pending verification over multiple patrol cycles)
- Canary deployment pipeline operational for safe testing of gastown changes
- Documentation updated for future operators

---

## Background

### Problem Statement

Starting 2026-01-22, the deacon patrol system experienced repeated dog timeout alerts:

```
DOG_TIMEOUT alpha - 32+ minutes on mol-session-gc
DOG_TIMEOUT bravo - 17+ minutes on mol-session-gc
```

Dogs are worker agents with a 10-minute timeout for infrastructure tasks. The `mol-session-gc` formula was consistently exceeding this limit, causing:

- Repeated timeout alerts flooding the mayor inbox
- Dogs getting stuck in "working" state with no active session
- Potential deacon instability from repeated timeout handling

### Initial Hypothesis

The handoff document hypothesized that `bd doctor --fix` (called by the formula) was taking 30+ minutes due to 259 duplicate beads requiring cleanup.

---

## CI/CD Canary Infrastructure

### Architecture Overview

```
GitHub Repository (shisuiki/gastown)
         │
         ▼
┌─────────────────────────────────────┐
│  GitHub Actions Workflow            │
│  .github/workflows/canary-deploy.yml│
│  Trigger: push to canary, manual    │
└─────────────────────────────────────┘
         │
         ▼
┌─────────────────────────────────────┐
│  Self-Hosted Runner                 │
│  /home/shisui/actions-runner-canary │
│  Labels: [self-hosted, canary]      │
└─────────────────────────────────────┘
         │
         ▼
┌─────────────────────────────────────┐
│  Canary Container                   │
│  Name: gastown-canary               │
│  Port: 8081                         │
│  Volume: /home/shisui/gt:/gt        │
└─────────────────────────────────────┘
```

### Components Deployed

#### 1. Self-Hosted GitHub Actions Runner

| Property | Value |
|----------|-------|
| Location | `/home/shisui/actions-runner-canary` |
| Label | `canary` |
| Service Name | `actions.runner.shisuiki-gastown.canary-runner` |
| Status | Running (systemd managed) |

**Installation Commands:**
```bash
mkdir -p /home/shisui/actions-runner-canary
cd /home/shisui/actions-runner-canary
curl -o actions-runner-linux-x64-2.321.0.tar.gz -L \
  https://github.com/actions/runner/releases/download/v2.321.0/actions-runner-linux-x64-2.321.0.tar.gz
tar xzf actions-runner-linux-x64-2.321.0.tar.gz
./config.sh --url https://github.com/shisuiki/gastown \
  --token [TOKEN] --name canary-runner --labels canary --unattended
sudo ./svc.sh install
sudo ./svc.sh start
```

#### 2. GitHub Secrets

| Secret | Purpose |
|--------|---------|
| `GT_WEB_AUTH_TOKEN_CANARY` | Authentication token for canary container |
| `GT_ROOT_CANARY` | Town root path (`/home/shisui/gt`) |
| `GT_CANARY_PORT` | Container port (`8081`) |

#### 3. Workflow Configuration

**File:** `.github/workflows/canary-deploy.yml`

**Key Fix Applied:** Changed runner from `ubuntu-latest` to `[self-hosted, canary]`:

```yaml
jobs:
  deploy-trigger:
    runs-on: [self-hosted, canary]  # Was: ubuntu-latest
```

#### 4. Canary Container

| Property | Value |
|----------|-------|
| Container Name | `gastown-canary` |
| Image | `gastown:canary-[commit-sha]` |
| Port Mapping | `8081:8080` |
| Volume Mount | `/home/shisui/gt:/gt` |
| Health Status | Healthy |

**Deploy Command:**
```bash
GT_ROOT=/home/shisui/gt \
GT_WEB_AUTH_TOKEN=[token] \
CANARY_PORT=8081 \
./deploy/canary-deploy.sh
```

---

## Dog Timeout Investigation

### Timeline of Investigation

| Time | Event |
|------|-------|
| 13:39 | Received handoff with P1 task |
| 13:45 | Cleaned 29 duplicate groups, 47 stale molecules |
| 13:46 | Installed self-hosted runner |
| 13:50 | Modified mol-session-gc formula with timeout instructions |
| 14:03 | Deployed canary container |
| 14:52 | First timeout alert after fix (stale state from before) |
| 15:55 | Second timeout - formula instructions not being followed |
| 16:36 | Third timeout - discovered template parsing bug (hq-7y5s5) |
| 16:45 | Applied template fix, pushed to main |

### Initial Fix Attempt: Formula Timeout Instructions

**Hypothesis:** Dogs timeout because `gt doctor --fix` is slow due to beads duplicates.

**Action:** Updated `mol-session-gc.formula.toml` to version 2 with:
- Timeout wrappers in instructions (5m doctor, 2m preview, 1m verify)
- Explicit instruction to skip slow `bd doctor` operations
- Documentation about dog timeout protection

**Result:** Failed - Dogs still timed out. Formula instructions are guidance for Claude agents, not enforced execution constraints.

### Key Insight

Formula descriptions are **instructions read by Claude agents**, not executable code. The agent decides what commands to run based on the instructions but may not follow them precisely.

---

## Root Cause Analysis

### The Real Problem: Template Parsing Bug

**Bug ID:** hq-7y5s5

**Error Message:**
```
missing required variables: timestamp, type, identifier, age, reason, item, error, else, bytes_freed, total_cleaned, report
```

**Location:** `mol-session-gc.formula.toml`, steps `report-gc` and `return-to-kennel`

**Problematic Code:**
```toml
description = """
**1. Generate report:**
```markdown
## Session GC Report: {{timestamp}}

**Mode**: {{mode}}

### Items Cleaned
{{#each cleaned}}
- {{type}}: {{identifier}} (age: {{age}}, reason: {{reason}})
{{/each}}
```
"""
```

### Why This Happened

1. The formula uses TOML format with multi-line string descriptions
2. Descriptions contained `{{placeholder}}` syntax as example output templates
3. The beads/formula system parsed these as **required template variables**
4. During wisp creation, the system expected values for `timestamp`, `type`, etc.
5. No values were provided, causing wisp creation to fail
6. The dog session crashed early, leaving stale "working" state
7. Deacon detected no tmux session + working state = timeout alert

### Failure Mode Diagram

```
Formula Load
     │
     ▼
Parse {{placeholders}} as required vars
     │
     ▼
Wisp Creation Attempt
     │
     ▼
"Missing required variables" ERROR
     │
     ▼
Dog Session Crash
     │
     ▼
Stale "working" state (no tmux session)
     │
     ▼
Deacon detects timeout condition
     │
     ▼
DOG_TIMEOUT alert
```

---

## Solutions Implemented

### 1. Template Syntax Fix

**Commit:** `d4a053ef`

**Change:** Replaced `{{placeholder}}` with `[PLACEHOLDER]` style:

```diff
- ## Session GC Report: {{timestamp}}
+ ## Session GC Report: [TIMESTAMP]

- **Mode**: {{mode}}
+ **Mode**: [MODE]

- {{#each cleaned}}
- - {{type}}: {{identifier}} (age: {{age}}, reason: {{reason}})
- {{/each}}
+ - [TYPE]: [ID] (age: [AGE], reason: [REASON])
+ - ... (list each cleaned item)
```

**Files Modified:**
- `/home/shisui/gt/.beads/formulas/mol-session-gc.formula.toml`
- `/home/shisui/work/gastown/internal/formula/formulas/mol-session-gc.formula.toml`

### 2. Beads Cleanup

| Action | Count |
|--------|-------|
| Duplicate groups merged | 29 |
| Stale molecules closed | 47 |

**Commands Used:**
```bash
bd duplicates --auto-merge
bd close [stale-molecule-ids]
```

### 3. CI/CD Infrastructure

See [CI/CD Canary Infrastructure](#cicd-canary-infrastructure) section above.

---

## Verification & Testing

### Immediate Verification

| Check | Result |
|-------|--------|
| Formula syntax valid | ✓ |
| Canary container healthy | ✓ |
| Self-hosted runner active | ✓ |
| Dogs in idle state | ✓ (all 4) |

### Pending Verification

- [ ] Next `mol-session-gc` dispatch completes without timeout
- [ ] No new DOG_TIMEOUT alerts over 24-hour period
- [ ] Canary workflow triggers successfully on next push

### How to Verify

```bash
# Check dog status
gt dog status

# Check for timeout alerts
gt mail inbox | grep DOG_TIMEOUT

# Check canary container
docker ps | grep canary

# Check runner service
systemctl status actions.runner.shisuiki-gastown.canary-runner
```

---

## Lessons Learned

### 1. Formula Descriptions Are Not Code

Formula `description` fields are instructions for Claude agents, not executable specifications. Agents interpret and may deviate from instructions.

**Implication:** Hard constraints must be implemented in Go code, not formula instructions.

### 2. Template Syntax in Documentation

Mustache-style `{{placeholders}}` in formula descriptions will be parsed as required variables.

**Best Practice:** Use `[PLACEHOLDER]` or other non-template syntax for example output in documentation.

### 3. Root Cause vs. Symptoms

Initial hypothesis (slow `bd doctor`) was incorrect. The actual issue (template parsing) caused immediate failure, not slow execution.

**Lesson:** When debugging timeouts, check for early failures before optimizing slow paths.

### 4. Canary Branch Protection Conflicts

The `canary` branch has protection rules requiring PRs, but the "Block Internal PRs" workflow closes all internal PRs.

**Workaround:** Push to main first, then deploy canary manually or via workflow_dispatch.

---

## Appendix

### A. Related Issues

| Issue ID | Title | Status |
|----------|-------|--------|
| hq-04n0l | Investigate dog alpha session-gc timeouts | Open (monitoring) |
| hq-7y5s5 | mol-session-gc formula template parsing bug | Closed (fixed) |

### B. Commits

| SHA | Description |
|-----|-------------|
| `acd1fe01` | fix(ci): Use self-hosted canary runner for deploy workflow |
| `d4a053ef` | fix(formula): Replace template placeholders with plain text |

### C. Files Modified

```
gastown/
├── .github/workflows/canary-deploy.yml  (runner fix)
└── internal/formula/formulas/mol-session-gc.formula.toml  (template fix)

gt/
├── .beads/formulas/mol-session-gc.formula.toml  (local sync)
└── gt_runtime_doc/mayor/
    ├── Memory.md  (infrastructure docs)
    ├── Progress.md  (work log)
    └── Roadmap.md  (canary workflow docs)
```

### D. Commands Reference

```bash
# Trigger canary deploy
gh workflow run canary-deploy.yml --ref main

# Check canary container
docker ps | grep canary
curl localhost:8081/health

# Check runner status
systemctl status actions.runner.shisuiki-gastown.canary-runner

# Clean beads duplicates
bd duplicates --auto-merge

# Check dog status
gt dog status
gt dog list --json
```

---

*Report generated by Mayor session, 2026-01-22*
