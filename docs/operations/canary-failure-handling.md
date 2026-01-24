---
type: runbook
status: active
owner: "unowned"
audience: oncall
applies_to:
  repo: gastown
  branch: canary
last_validated: "unknown"
ttl_days: 30
next_review: "unknown"
source_of_truth:
  - "scripts/canary-command-retry.sh"
  - "scripts/canary-alert.sh"
  - ".github/workflows/canary-deploy.yml"
  - "scripts/canary-docker-exec.sh"
---

# Canary Failure Handling and Recovery

Step 10 of the `hq-vsa3v` Canary Deploy Infrastructure epic (`hq-sic13`) focuses on understanding what can go wrong when the canary workflow runs and automating as much of the response as possible. This document references the Docker exec trigger workflow from Step 5 and extends it with failure classification, retry/exponential-backoff guidance, rollback paths, and alerting hooks.

## Failure scenarios

| Category | Detection | Response |
| --- | --- | --- |
| CI failures (build/test/lint) | GitHub Actions job `deploy-trigger` exits non-zero during the health/config/migrate/smoke steps. | Retry up to `CANARY_RETRY_ATTEMPTS` with exponential backoff; if the final attempt still fails, allow the `rollback` job to run and send alerts. Document flaky suites and disable retry for deterministic failures (see Retry policy). |
| Docker daemon errors | `scripts/canary-docker-exec.sh` fails before the inner command runs (Docker not installed, daemon unreachable, permission denied). | Do **not** retry—these usually require operator fixes. Fail fast, trigger rollback, and surface a critical alert. |
| Container startup failures | `ensure_container_running()` cannot start `CANARY_CONTAINER` even after one `docker start` attempt. | Treat as transient by default (retry once), but require manual verification of the host/container state if the issue persists. If restart repeatedly fails, rollback job runs and alerts escalate to on-call. |
| Application/test flakiness | One of the inner health/config/migrate/smoke commands exits non-zero despite the container running. | Allow up to `CANARY_RETRY_ATTEMPTS` with delay (see Retry policy). If fluctuates between success/failure, mark as flaky in `docs/operations/canary-failure-handling.md` and require manual run (stop automatic retries until fixed). |

## Retry policy

The GitHub workflow relies on `scripts/canary-command-retry.sh` to enforce retries and exponential backoff before giving up:

- `CANARY_RETRY_ATTEMPTS` (default `3`): how many times to run a particular command before failing the job.
- `CANARY_RETRY_BACKOFF_SECONDS` (default `10`): the initial delay before the second attempt; the delay doubles after each failure (10, 20, 40, ...).
- `CANARY_RETRY_WHITELIST` (optional comma-separated list of commands, e.g., `health,migrate`): commands that should participate in retries; other commands fail fast.
- `CANARY_RETRY_BLACKLIST` (optional): commands that should never retry (e.g., `rollback`).

**When not to retry**:
1. Docker CLI or daemon missing/permission errors (non-zero exit before `run_inner_command`).
2. `ensure_container_running()` hits its restart attempt limit.
3. The workflow already retried and the failure is deterministic in logs (e.g., missing migration file).
4. `CANARY_RETRY_FLAG=manual` is set in workflow input, forcing a single run.

The script logs each attempt and emits the final failure reason so downstream jobs (rollback & alert) can explain what happened.

## Rollback procedures

### Automated rollback
- The `rollback` GitHub job runs whenever `deploy-trigger` fails (`if: failure()`), reuses `scripts/canary-docker-exec.sh --command rollback`, and observes the same container validation + `timeout` guardrails.
- The job requires `CANARY_ROLLBACK_COMMANDS` (default: `git reset --hard origin/main;;gt config reload`) so operators can define their own flow.
- Rollback steps are constrained by `CANARY_COMMAND_TIMEOUT` and share the same container health safety checks as the main workflow.

### Manual rollback
1. Attach to the canary host (SSH) and verify the container state:
   ```bash
docker ps --filter "name=${CANARY_CONTAINER:-gastown-canary}"
```
2. Run the rollback commands in sequence using the helper:
   ```bash
CANARY_ROLLBACK_COMMANDS='git reset --hard origin/main;;gt config reload' scripts/canary-docker-exec.sh --command rollback
```
3. If the container is still unhealthy, stop and remove it before pulling a fresh image/service up:
   ```bash
docker stop $CONTAINER && docker rm $CONTAINER
docker run ...
```
4. Document observed logs in `docs/operations/canary-failure-handling.md` under the “Manual recovery notes” section so future runs benefit.

## Alerting plan

Alerting is handled by `scripts/canary-alert.sh`, which runs in a dedicated GitHub Actions job (`alert-on-failure`) that depends on `rollback` and inherits failure context:

- Environment variables:
  - `CANARY_ALERT_EMAIL`: sendmail-style address to notify.
  - `CANARY_ALERT_WEBHOOK`: HTTP endpoint (Slack, PagerDuty, etc.) that accepts a JSON payload.
  - `CANARY_ALERT_LEVEL`: defaults to `warning`; set to `critical` when retries were exhausted or rollback failed.
- Behavior:
  - If neither email nor webhook is configured, the script prints a summary and exits `0` so the workflow can finish gracefully.
  - When configured, it posts a summary that includes the failed command, final exit code, retry count, and `GITHUB_RUN_ID`/`GITHUB_RUN_NUMBER` for traceability.

**Alert triggers**:
1. Deploy failure → `deploy-trigger` fails → `rollback`+`alert-on-failure` jobs run.
2. Rollback failure → `scripts/canary-docker-exec.sh --command rollback` returns non-zero → `alert-on-failure` marks level `critical` and includes rollback logs.
3. Repeated failures (e.g., all retry attempts exhausted) set `CANARY_ALERT_LEVEL=critical` and include the retry stack in the payload.

Operators can read the alert summary to decide whether to pause the canary branch or open a manual incident.

## Manual escalation
- If alerts keep firing for the same issue, disable the canary workflow (remove the `canary` branch trigger) and hand the incident to the team via email/Slack, referencing bead `hq-sic13`.
- Log the failure and mitigation steps under the “Manual recovery notes” section in this document so future engineers inherit the context.

## Preconditions
- `scripts/canary-command-retry.sh` and `scripts/canary-alert.sh` are executable.
- `CANARY_RETRY_ATTEMPTS`, `CANARY_ALERT_EMAIL`, and `CANARY_ALERT_WEBHOOK` are configured for the environment.
- Operators are subscribed to the alert channel referenced in `CANARY_ALERT_WEBHOOK`.

## Steps
1. When `deploy-trigger` fails, read `canary-failure-context.txt` (or the run output from `scripts/canary-command-retry.sh`) to capture the failed command, exit code, and retry count.
2. Allow `.github/workflows/canary-deploy.yml` to run the `rollback` job, which executes `scripts/canary-docker-exec.sh --command rollback` with the configured `CANARY_ROLLBACK_COMMANDS`.
3. If rollback succeeds, close the canary bead; if it fails, notify stakeholders through the alert job and escalate manually.
4. When repeated failures occur (e.g., all retry attempts exhausted), pause the `canary` branch trigger (remove the push trigger or set `CANARY_PAUSE=1`) and file an incident referencing bead `hq-sic13`.
5. Document the investigation outcomes and manual steps in this document under a “Manual recovery notes” section with the command outputs.

## Verification
- `scripts/canary-command-retry.sh --command health --no-retry`
- `scripts/canary-alert.sh --message "test alert" --context canary-failure-context.txt`
- `cat canary-failure-context.txt`
- `docker logs gastown-canary --tail 200`
