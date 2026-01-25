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
  - "deploy/canary/compose.sh"
  - "deploy/canary/manifest.sh"
  - "deploy/canary-manifest.yaml"
---

# Canary Env-Config Injection and Deploy Metadata

This document defines the env-config injection method and deployment metadata
requirements for canary deployments.

## Decision: Bind-Mount a Pinned env-config Worktree

**Chosen method**: Create a git worktree at a specific env-config SHA and
bind-mount it read-only into the canary container at runtime.

### Pros
- Fast: no image rebuild required for config updates.
- Explicit pairing: deploy uses a pinned env-config SHA.
- Auditable: worktree path and manifest capture the exact config version.

### Cons
- Requires env-config repo on the host.
- Container depends on host-mounted filesystem.
- Must manage worktree lifecycle and retention.

### Alternatives Considered
- **Bake into image**: simpler runtime but forces rebuilds on config changes.
- **Inject at runtime** (env vars, secrets): good for small configs but not
  suitable for larger structured config trees.

## Deployment Composition Workflow

Use `deploy/canary/compose.sh` to pin env-config and record a deploy manifest.

```bash
export GASTOWN_SHA=<sha>
export ENV_CONFIG_SHA=<sha>
export IMAGE_TAG=<tag>
deploy/canary/compose.sh --gastown-sha "$GASTOWN_SHA" \
  --env-config-sha "$ENV_CONFIG_SHA" \
  --image-tag "$IMAGE_TAG"
```

Outputs:
- A pinned env-config worktree at:
  `~/gt/deployments/canary/env-config/<env-config-sha>`
- A deploy manifest in:
  `~/gt/deployments/canary/manifests/`
- A summary env file at:
  `~/gt/deployments/canary/last-deploy.env`

The container runtime should bind-mount `ENV_CONFIG_PATH` read-only.

### Compatibility Guard (Optional)

If the env-config worktree contains `compat.json` with a `gastown_sha` field,
`compose.sh` enforces an exact match. On mismatch, the deploy must be aborted
and a tracking issue filed describing the mismatch.

## Deploy Manifest Schema

Each manifest records:
- `timestamp` (UTC, RFC3339)
- `gastown_sha`
- `env_config_sha`
- `image_tag`
- `deploy_result` (pending/success/failure)

## Audit Trail and Retention

Manifests are retained in `~/gt/deployments/canary/manifests/` with a default
retention of 10 records. Tune with `CANARY_DEPLOY_RETENTION`.

### Query Examples

List recent deploys (requires `jq`):
```bash
deploy/canary/manifest.sh list --limit 10
```

Show a specific manifest:
```bash
deploy/canary/manifest.sh show <path>
```

Get the latest manifest path:
```bash
deploy/canary/manifest.sh latest
```

## Preconditions
- The env-config repo (e.g., `~/gt/env-config`) is cloned and hosts the SHA referenced by `CANARY_ENV_CONFIG_SHA`.
- `deploy/canary/compose.sh`, `deploy/canary/manifest.sh`, and `deploy/canary-manifest.yaml` exist and are executable.
- `CANARY_IMAGE_TAG` and `GT_WEB_AUTH_TOKEN` are exported before running the composition script.

## Steps
1. Run `deploy/canary/compose.sh --gastown-sha <sha> --env-config-sha <env_sha> --image-tag <tag>` to pin the env-config worktree and record the manifest.
2. Confirm that `~/gt/deployments/canary/env-config/<env_sha>` exists and is populated with the expected files.
3. Bind-mount the generated `ENV_CONFIG_PATH` into `gastown-canary` or package it into the deploy manifest used by `deploy/canary-deploy.sh`.
4. Periodically prune old manifests with `deploy/canary/manifest.sh clean --keep 10` (or adjust `CANARY_DEPLOY_RETENTION` as needed).

## Verification
- `deploy/canary/manifest.sh list --limit 5`
- `deploy/canary/manifest.sh latest | grep env_config_sha`
- `test -f ~/gt/deployments/canary/last-deploy.env`
