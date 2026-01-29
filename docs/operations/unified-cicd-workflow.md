---
type: runbook
status: active
owner: "unowned"
audience: ops
applies_to:
  repo: gastown
  branch: canary
last_validated: "unknown"
ttl_days: 30
next_review: "unknown"
source_of_truth:
  - ".github/workflows/canary-deploy.yml"
  - "deploy/canary-deploy.sh"
---

# Unified CI/CD Workflow

This document describes the integrated CI/CD pipeline that coordinates changes across three repositories:
- **gastown** (application code)
- **GTRuntime** (formulas in `.beads/` + runtime docs in `gt_runtime_doc/`)

## Architecture Overview

```
┌─────────────────────────────────────────────────────────────────┐
│                    Unified Canary Pipeline                       │
├─────────────────────────────────────────────────────────────────┤
│                                                                  │
│   GTRuntime repo (anantheparty/GTRuntime)                       │
│   ├── .beads/formulas/     ─┐                                   │
│   └── gt_runtime_doc/      ─┼── Push to canary branch           │
│                             │                                    │
│                             ▼                                    │
│                    canary-sync.yml                               │
│                    (sync gt-canary workspace)                    │
│                             │                                    │
│                             ▼                                    │
│                    repository_dispatch ──────────────────────┐  │
│                                                              │  │
│   gastown repo (shisuiki/gastown)                           │  │
│   └── Push to canary branch ─────────────────────────────┐  │  │
│                                                          │  │  │
│                                                          ▼  ▼  │
│                                                   canary-deploy.yml
│                                                          │      │
│                                                          ▼      │
│                                              gastown-canary     │
│                                              (port 8081)        │
│                                              mounts gt-canary   │
│                                                                  │
└─────────────────────────────────────────────────────────────────┘
```

## Workspace Structure

| Path | Purpose | Branch |
|------|---------|--------|
| `/home/shisui/gt` | Production GTRuntime | master |
| `/home/shisui/gt-canary` | Canary GTRuntime (worktree) | canary |
| `/home/shisui/work/gastown` | gastown source | main/canary |

The canary workspace is a git worktree, sharing the same git objects as production but tracking the `canary` branch.

## Triggers

### GTRuntime Changes (Formulas/Runtime Docs)

1. Developer pushes to `canary` branch in GTRuntime
2. `canary-sync.yml` workflow triggers:
   - Syncs `/home/shisui/gt-canary` worktree
   - Sends `repository_dispatch` to gastown
3. gastown's `canary-deploy.yml` receives dispatch:
   - Validates canary workspace
   - Deploys canary container with canary GTRuntime

### gastown Changes (Application Code)

1. Developer pushes to `canary` branch in gastown
2. `canary-deploy.yml` triggers directly:
   - Validates canary workspace
   - Builds and deploys canary container

## Workflows

### GTRuntime Workflows

#### canary-sync.yml
- **Triggers**: Push to canary branch (paths: `.beads/formulas/**`, `gt_runtime_doc/**`)
- **Actions**:
  1. Syncs canary worktree: `git fetch && git reset --hard origin/canary`
  2. Dispatches to gastown repository

#### formula-validation.yml
- **Triggers**: PRs and pushes affecting `.beads/formulas/**`
- **Actions**:
  1. Validates TOML syntax
  2. Checks for problematic `{{var}}` template syntax (DOG_TIMEOUT prevention)
  3. Verifies required fields (name, version)
  4. Warns on high-risk formula changes (deacon, witness, shutdown)

### gastown Workflows

#### canary-deploy.yml
- **Triggers**:
  - Push to canary branch
  - `repository_dispatch` (type: `dependency-updated`)
  - Manual dispatch
- **Actions**:
  1. Validates canary workspace exists
  2. Logs dependency versions (GTRuntime SHA)
  3. Runs deployment steps (health, config, migrate, smoke)

## Setup Instructions

### Initial Setup

```bash
# 1. Create canary branch in GTRuntime (if not exists)
cd /home/shisui/gt
git checkout -b canary
git push -u origin canary
git checkout master

# 2. Create canary worktree
git worktree add /home/shisui/gt-canary canary

# Or use the setup script:
cd /home/shisui/work/gastown
./deploy/setup-canary-workspace.sh
```

### Required Secrets

| Secret | Repository | Purpose |
|--------|------------|---------|
| `GASTOWN_DISPATCH_TOKEN` | GTRuntime | PAT to trigger gastown workflows |
| `GT_WEB_AUTH_TOKEN_CANARY` | gastown | Web API authentication for canary |

## Promotion Process

### Promoting Formula/Runtime Doc Changes

```bash
# 1. Verify canary is stable (check gastown-canary health)
curl -H "Authorization: Bearer $TOKEN" http://localhost:8081/api/health

# 2. Merge canary to master in GTRuntime
cd /home/shisui/gt
git checkout master
git merge canary --no-ff -m "Promote canary to master"
git push origin master

# 3. Sync production workspace
# (Already on master, just pulled)
```

### Promoting gastown Changes

```bash
# 1. Verify canary is stable
./deploy/canary-validate.sh

# 2. Create PR or merge directly
cd /home/shisui/work/gastown
git checkout main
git merge canary --no-ff -m "Promote canary to main"
git push origin main
```

## Rollback Procedures

### Formula Rollback

```bash
# Reset canary workspace to master
cd /home/shisui/gt-canary
git fetch origin master
git reset --hard origin/master

# Force canary branch to match master
git push origin master:canary --force
```

### gastown Rollback

```bash
# Use rollback script
./deploy/canary-rollback.sh

# Or manually
docker stop gastown-canary
docker start gastown-canary  # Uses previous image from state file
```

## Monitoring

### Deployment State

```bash
# View current deployment info
cat /home/shisui/gt-canary/logs/canary-deploy.json

# Check container labels
docker inspect gastown-canary --format '{{.Config.Labels}}'
```

### Validation

```bash
# Run full validation suite
./deploy/canary-validate.sh
```

## Troubleshooting

### Canary workspace not syncing

1. Check worktree status: `git -C /home/shisui/gt-canary status`
2. Verify branch: `git -C /home/shisui/gt-canary branch`
3. Manual sync: `cd /home/shisui/gt-canary && git fetch origin canary && git reset --hard origin/canary`

### repository_dispatch not triggering

1. Verify `GASTOWN_DISPATCH_TOKEN` secret is set in GTRuntime repo
2. Check token has `repo` scope
3. Check workflow logs in GTRuntime for dispatch errors

### Formula validation failing

1. Check for `{{var}}` syntax - use `[PLACEHOLDER]` instead
2. Verify TOML syntax with: `python -c "import toml; toml.load('file.toml')"`
3. Ensure required fields: `name`, `version`

## Related Documentation

- [Canary Deploy Checklist](canary-deploy-checklist.md)
- [Canary Promotion](canary-promotion.md)
- [Canary Failure Handling](canary-failure-handling.md)

## Preconditions
- `GASTOWN_DISPATCH_TOKEN` secret exists and is scoped to trigger gastown workflows.
- `canary` branch worktrees (`/home/shisui/gt-canary` and `/home/shisui/work/gastown`) exist and track actual repos.
- `canary-deploy.yml`, `canary-deploy.sh`, and `canary-validate.sh` are present in the repo.

## Steps
1. Push changes to `canary` in GTRuntime or gastown depending on the artifact (formulas vs. application code).
2. Allow `canary-sync.yml` (GTRuntime) to sync the worktree and dispatch `repository_dispatch` events to gastown.
3. Let `canary-deploy.yml` pick up the push or dispatch, validate the workspace, and run deployment steps.
4. If deployment fails, follow `docs/operations/canary-docker-exec-workflow.md` and `docs/operations/canary-failure-handling.md` for retry/alert protocols.
5. After stabilization, promote the change with `docs/operations/canary-promotion.md` guidance.

## Verification
- `gh workflow view canary-deploy.yml --json state`
- `cat /home/shisui/gt-canary/logs/canary-deploy.json | jq .deploy_result`
- `deploy/canary-validate.sh --help`
