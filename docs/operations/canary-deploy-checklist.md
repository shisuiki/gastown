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
  - "deploy/canary-rollback.sh"
---

# Canary Deploy Checklist

Use this checklist when rolling the canary container.

## Preconditions

- [ ] Latest container image available in GHCR (`ghcr.io/<org>/<repo>`).
- [ ] `bd` is present in the container (verify `bd version`).
- [ ] Workspace volume mounted (contains `mayor/town.json`).
- [ ] `GT_WEB_AUTH_TOKEN` set and `GT_WEB_ALLOW_REMOTE=1` for remote access.

## Deploy Steps

- [ ] Pull the image tag (sha or `canary`).
- [ ] Start the container with port 8080 published and workspace mounted.
- [ ] Verify `gt gui` stays up (container does not exit).
- [ ] Run the health check (`gt version` or HTTP `GET /api/version`).
- [ ] Smoke test: open the web UI and load the dashboard.

## Beads Tracking

Track canary deploys in beads so they are auditable and visible to the team.

- [ ] Ensure `deploy/canary-deploy.sh` runs with bead recording enabled:
  ```bash
  export CANARY_RECORD_BEAD=1
  ./deploy/canary-deploy.sh
  ```
- [ ] If you did not use the deploy script, record manually:
  ```bash
  CANARY_RESULT=success ./scripts/canary-record-bead.sh
  ```
- [ ] Confirm the event exists (`bd list --type=event --event-category=canary.deploy --limit 1`).

## Rollback Criteria

- [ ] Web UI fails to start or crashes repeatedly.
- [ ] Health check fails after two retries.
- [ ] Critical errors in logs (`docker logs <container>`).

If rollback is required, stop the container and redeploy the previous image tag.

## Steps
1. Confirm prerequisites: `deploy/canary-deploy.sh --help` returns usage, the latest image tag exists in GHCR, and env vars such as `GT_WEB_AUTH_TOKEN`/`CANARY_PORT` are exported.
2. Run `deploy/canary-deploy.sh` (with `VALIDATE_CANARY=1`) to build/pull the image and start `gastown-canary` with `/gt` bind-mounted.
3. After the container is up, execute the configured validation steps (`gt version`, `curl http://localhost:8081/api/version`, `open http://localhost:8080/` in a browser if allowed) while watching container logs.
4. Record each milestone in the canary bead: `bd update <bead-id> --status=... --note "validation step passed"`.
5. If anything fails, stop the container, redeploy the previous image, and capture logs (`docker logs --tail 200 gastown-canary`) before closing the bead.

## Verification
- `docker ps --filter "name=gastown-canary" --format '{{.Names}}\t{{.Status}}'`
- `curl -fsS http://localhost:8081/api/version`
- `bd show <bead-id> --json` (validate tracking metadata exists)
- `deploy/canary-rollback.sh --help` (ensure rollback helper is available)
