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

- [ ] Create a bead for the deploy:
  ```bash
  bd new -t task -p 1 "Canary deploy $(date +%Y-%m-%d)"
  ```
- [ ] Update the bead with status/notes at each milestone (`bd update <id> --status=...`).
- [ ] Close the bead after the smoke test passes.

## Rollback Criteria

- [ ] Web UI fails to start or crashes repeatedly.
- [ ] Health check fails after two retries.
- [ ] Critical errors in logs (`docker logs <container>`).

If rollback is required, stop the container and redeploy the previous image tag.
