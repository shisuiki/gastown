# Canary Deploy Workflow

This document defines the canary deployment flow for Gas Town.

## Canary Host

- Hostname: `shisuiki`
- Workspace root: `/home/shisui/gt`
- Docker: installed (`docker --version`), accessible via `sudo docker`.

If non-sudo Docker access is desired, add the deployment user to the `docker`
 group and re-login.

## Container Naming

- Container name: `gastown-canary`
- Image tag: `gastown:canary-<git-sha>`
- Port: `8081` (defaults; configurable with `CANARY_PORT`)

## Manifest Pairing

`deploy/canary-manifest.yaml` pins the env-config ref used for canary deploys.
The deploy workflow uses the current `canary` commit for gastown and the pinned
env-config ref for configuration.

See `docs/MAYOR-CREW-DEPLOY.md` for the standard mayor → crew request and status
update templates.

## Deploy Script

`deploy/canary-deploy.sh` builds a Docker image and runs the canary container.
Required environment variables:

- `GT_WEB_AUTH_TOKEN`: token for the web UI
- `GT_ROOT` (optional): defaults to `/home/shisui/gt`
- `CANARY_PORT` (optional): defaults to `8081`

The script records metadata at:

- `/home/shisui/gt/logs/canary-deploy.json`
- `/home/shisui/gt/logs/canary-deploy.env`

## GitHub Actions Workflow

`.github/workflows/canary-deploy.yml` triggers on pushes to `canary` and runs on
self-hosted runners labeled `canary`. It checks out env-config at the pinned ref
from the manifest and runs the deploy script.

Required secrets (configured on the repo):

- `GT_WEB_AUTH_TOKEN_CANARY`
- `GT_ROOT_CANARY` (optional; defaults to `/home/shisui/gt`)
- `GT_CANARY_PORT` (optional; defaults to `8081`)

## Rollback

Use the rollback helper after a failed or bad deploy:

```bash
export GT_WEB_AUTH_TOKEN=...
./deploy/canary-rollback.sh
```

The rollback script uses `logs/canary-deploy.env` to restart the previous image.
