---
type: evergreen
status: active
owner: "unowned"
audience: dev
applies_to:
  repo: gastown
  branch: main
last_validated: "unknown"
source_of_truth: []
---

# Mayor → Crew Deploy Workflow

This document defines the standard request and status formats for canary deploy
work. Use it for mayor-to-crew handoffs and operational reporting.

## Deploy Request Template (Mayor → Crew)

Use this template when assigning deploy work to a crew member.

```
Title: Canary deploy for <service> (<env>)
Priority: P0/P1/P2
Owner: mayor
Assignee: <rig>/crew/<name>

Objective
- Deploy <service> to <environment> (canary or production)

Inputs
- Gastown ref: <commit SHA or tag>
- GTRuntime ref: <commit SHA or HEAD>
- Branch: <canary|main>

Host / Runner
- Canary host: <hostname>
- Runner label: <label>
- Docker access: <sudo|group|socket>

Prechecks
- [ ] Branch protection passes for target branch
- [ ] Runner is online and labeled correctly
- [ ] Docker daemon reachable on host

Execution
- [ ] Run canary workflow or script
- [ ] Verify health checks
- [ ] Record deploy metadata

Rollback
- Rollback criteria: <define>
- Rollback command: deploy/canary-rollback.sh

Notes
- Special instructions or known risks
```

## Status Update Template (Crew → Mayor)

```
Status: <blocked|in_progress|done>

Changes
- Commits: <hashes and summaries>
- Files: <paths changed>

Tests
- <command>: <result>

Deploy
- Host: <hostname>
- Workflow/run: <link or id>
- Result: <success|failed>
- Metadata: <path to deploy metadata>

Issues / Failures
- <what failed>
- <recovery/rollback actions>

Escalations
- <what needs mayor/human input>
```

## Escalation Rules

Escalate to the Mayor immediately if any of the following occur:

- Repo access blocked (gastown or GTRuntime).
- Docker daemon unavailable or permission denied on canary host.
- Self-hosted runner offline or missing required labels.
- Required secrets missing (e.g., `GT_WEB_AUTH_TOKEN_CANARY`).
- Deploy health checks fail and rollback is not successful.
- Migrations fail or data integrity is at risk.
- Canary host identity is unclear.

Escalate to the human if the Mayor cannot resolve:

- Infrastructure access cannot be granted.
- Host ownership or security approval is required.
- Production-impacting failure without safe rollback.

## Example Request

```
Title: Canary deploy for gastown (canary)
Priority: P1
Owner: mayor
Assignee: TerraNomadicCity/crew/Ptilopsis

Objective
- Deploy gastown canary container from canary branch

Inputs
- Gastown ref: refs/heads/canary
- GTRuntime ref: HEAD (mounted at /gt)
- Branch: canary

Host / Runner
- Canary host: shisuiki
- Runner label: canary
- Docker access: sudo

Execution
- Trigger workflow .github/workflows/canary-deploy.yml
- Verify container health + web UI
- Record deploy metadata in /home/shisui/gt/logs/

Rollback
- If health check fails, run deploy/canary-rollback.sh
```

## Scope
- Scope description pending.
