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
  - "scripts/canary-docker-exec.sh"
  - ".github/workflows/canary-deploy.yml"
---

# Canary Docker Exec Workflow

This workflow describes Step 5 of the `hq-vsa3v` Canary Deploy Infrastructure epic (`hq-c85he`). The goal is to trigger post-deploy sanity checks and maintenance commands inside the canary Docker container while surfacing failures through the deployment workflow. It also documents how to validate the canary container locally or on a staging host so that the command library can be exercised before the GitHub job runs.

## Build and Run
```bash
docker build -t gastown:canary .

# Start the container
docker run -d --name gastown-canary \
  -p 8080:8080 \
  -v /path/to/gt-workspace:/gt \
  -e GT_WEB_AUTH_TOKEN=changeme \
  -e GT_WEB_ALLOW_REMOTE=1 \
  gastown:canary
```

## Validate Binaries
```bash
docker exec gastown-canary gt version
docker exec gastown-canary bd version
```

## Logs and Health
```bash
docker logs -f gastown-canary

# HTTP health check
curl -H "Authorization: Bearer $GT_WEB_AUTH_TOKEN" http://localhost:8080/api/version
```

## Cleanup
```bash
docker stop gastown-canary
docker rm gastown-canary
```

## Prerequisites
- Docker CLI must be available inside the environment that runs the workflow (CI job, ops workstation, etc.).
- The canary container image is already running and reachable (default name `gastown-canary`, override via `CANARY_CONTAINER`).
- GitHub Actions secrets (if needed) are configured on the runner, since the workflow simply runs Docker commands and does not push code.

## Command library
| Command | Purpose | Default inner command |
| --- | --- | --- |
| `health` | HTTP-based health check for the canary app | `curl -fsS http://localhost:12121/_/health` |
| `config` | Reload configuration or feature flags without redeploy | `gt config reload` |
| `migrate` | Apply database migrations that accompany the deploy | `gt migrations apply` |
| `smoke` | A small smoke test that exercises the public endpoint | `curl -fsS http://localhost:41241/_/ping` |

The wrapper script `scripts/canary-docker-exec.sh` (described below) enforces this command library and exposes the commands via the `--command` (`health`, `config`, `migrate`, `smoke`, or `rollback`) flag.

## Wrapper behavior
- Usage: `scripts/canary-docker-exec.sh --container <name> --timeout <seconds> --command <name>`.
- It inspects the requested container; if the container is not running it attempts a single `docker start`. If that fails, the script exits with status `1` so the workflow can roll back.
- It runs the mapped command inside the container via `timeout` (default 300 s). If the timed process exits non-zero after the command map is resolved, the script surfaces the failure back to GitHub Actions for automatic rollback.
- Environment overrides are available: `CANARY_COMMAND_TIMEOUT` (e.g., `CANARY_COMMAND_TIMEOUT=180`).
- The script prints useful logs for every step so operators can trace which command failed.

## Error handling and rollback triggering
- Non-zero exit codes bubble up from `scripts/canary-docker-exec.sh`, causing the GitHub Actions job to mark the deployment as failed.
- If the container is down, the wrapper attempts one restart before failing; repeated failure from `docker start` still leaves the job in the failed state.
- Timeout handling leverages the `timeout` coreutils utility; when the timer expires the container command is killed and the job is aborted.
- The GitHub workflow adds a `rollback` job that runs `scripts/canary-docker-exec.sh --command rollback`. This job uses `if: failure()` so it only runs when `deploy-trigger` exits abnormally.

## Workflow integration (`.github/workflows/canary-deploy.yml`)
1. **Trigger rules**: `on.push.branches: [canary]` plus `workflow_dispatch` so teams can retry manually.
2. **Jobs**:
   - `deploy-trigger` (runs on `ubuntu-latest`): checks out the repo, ensures the wrapper script is executable, and sequentially runs the four standard commands from the library via separate steps. A single failure short-circuits the job, relying on GitHub Actions to block merges and surface the log.
   - `rollback` (runs on `ubuntu-latest`, needs `deploy-trigger`, but only on failure): reuses the wrapper script with `--command rollback` to bring the canary container back to a known-good state. The actual rollback command(s) can be configured via `CANARY_ROLLBACK_COMMANDS` (defaulting to a logged placeholder) so operators can add the exact steps later.
3. **Environment propagation**: Workflow-level env variables (such as `CANARY_CONTAINER` and `CANARY_COMMAND_TIMEOUT`) make it easy to customize the wrapper without modifying the script.
4. **Artifacts/logging**: Each step logs the executed `docker exec` command and the exit code so postmortems can trace which standard check failed.

## Rollback customization
The `rollback` command runs inside the wrapper so it benefits from the same container validation and `timeout`. Operators can configure `CANARY_ROLLBACK_COMMANDS` (split by `;;` to allow multiple instructions) which the wrapper runs sequentially inside the container:

```
CANARY_ROLLBACK_COMMANDS='git reset --hard origin/main;;gt config reload'
```

If no override is provided, the script simply logs that the rollback hook ran and does not modify container state (safe no-op).

## Manual usage
To run a single command locally (for debugging or pre-flight), execute:

```bash
CANARY_CONTAINER=gastown-canary ./scripts/canary-docker-exec.sh --command health
```

This mirrors the steps the workflow performs and reuses the same error handling.

## References
- Step 5: Deploy trigger mechanism (Docker exec) – bead `hq-c85he`
- Canary Deploy Infrastructure epic `hq-vsa3v`

## Preconditions
- `scripts/canary-docker-exec.sh` is executable in the repo root.
- The canary container image is built and `gastown-canary` is ready to run on the host.
- Required tokens (e.g., `GT_WEB_AUTH_TOKEN`) and overrides (`CANARY_COMMAND_TIMEOUT`) are available for the helper.

## Steps
1. Inspect `docker ps --filter name=gastown-canary` to ensure the container can be restarted.
2. Run `scripts/canary-docker-exec.sh --command health --timeout 60` and confirm it returns exit `0`.
3. Sequentially invoke `scripts/canary-docker-exec.sh --command config`, `... --command migrate`, and `... --command smoke` to refresh config, run migrations, and exercise the public endpoint.
4. If any command fails, let the `.github/workflows/canary-deploy.yml` `rollback` job run `scripts/canary-docker-exec.sh --command rollback` with `CANARY_ROLLBACK_COMMANDS` defined.
5. Capture key logs (`docker logs gastown-canary --tail 200`) and attach them to the current bead for traceability.

## Verification
- `scripts/canary-docker-exec.sh --command health --timeout 60`
- `docker inspect gastown-canary --format '{{.State.Status}}'`
- `scripts/canary-docker-exec.sh --command rollback --timeout 120 CANARY_ROLLBACK_COMMANDS='echo rollback dry run'`
