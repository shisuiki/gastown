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
