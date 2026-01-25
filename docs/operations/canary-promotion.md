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
  - "deploy/canary-deploy.sh"
  - "deploy/canary-validate.sh"
  - "docs/MAYOR-CREW-DEPLOY.md"
---

# Canary Promotion to Main

This document defines the promotion checklist from `canary` to `main` for the
Gastown canary flow.

## Pass Criteria

Promotion is allowed only when ALL of the following are true:

- All required CI checks on `canary` are green.
- Canary deployment has a stability window of 24 hours with no P1/P2 incidents.
- Validation workflow (Step 7) has passed for the target release.
- Required approvals obtained (at least one reviewer + human approval for merge).

If tests are flaky or incomplete, block promotion and file a stabilization issue.

## Promotion Workflow (Manual + PR)

1. Confirm `canary` is up to date.
2. Verify canary deploy metadata is present (`/home/shisui/gt/logs/canary-deploy.json`).
3. Validate env-config ref remains pinned in `deploy/canary-manifest.yaml`.
4. Create a PR from `canary` to `main`.
5. Wait for required CI checks and approvals.
6. Human approval for final merge.
7. Merge PR (no direct pushes).
8. Record promotion metadata in the tracking bead.

Example PR creation (run from the repo):

```bash
BASE=main
HEAD=canary
TITLE="Promote canary to main"
BODY="Promotion after 24h stability."

gh pr create --base "$BASE" --head "$HEAD" --title "$TITLE" --body "$BODY"
```

## Regression Handling

A regression is any of the following within 24 hours of promotion:

- P1/P2 incidents attributable to the promoted change.
- Canary health checks that failed and were masked by retry.
- Major functional regression reported by operators.

### Revert Procedure

1. Identify the last known good commit (from canary deploy metadata).
2. Create a revert PR against `main`.
3. Merge the revert after approval.
4. Notify stakeholders and record the incident in the tracking bead.

Example revert:

```bash
git revert <bad_commit_sha>
git push origin HEAD
```

## Status Reporting

Crew should report promotion status using the template in
`docs/MAYOR-CREW-DEPLOY.md`.

## Preconditions
- `canary` branch has a 24h stability window with no P1/P2 incidents.
- CI for the current `canary` SHA is green.
- The deploy bead is closed, and `deploy/canary-deploy.sh`/`deploy/canary-validate.sh` last executed without critical errors.

## Steps
1. Confirm `canary` is in sync with the latest meta branch (`git checkout canary && git pull`).
2. Verify `~/gt/logs/canary-deploy.json` shows the expected `deploy_result=success`.
3. Create a PR from `canary` to `main` using `gh pr create --base main --head canary`.
4. Wait for required reviews and the human approval step; avoid merging until the approver handshake is complete.
5. Merge the PR once approvals and checks pass, then close the canary bead with notes referencing the promotion.

## Verification
- `git status -sb`
- `gh pr view --head canary --json state,title`
- `curl -fsS http://localhost:8081/api/version`
- `bd show <promotion-bead-id>`
