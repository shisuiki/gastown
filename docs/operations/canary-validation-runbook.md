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
  - "scripts/canary-command-retry.sh"
  - "deploy/canary-validate.sh"
  - "deploy/canary-deploy.sh"
---

# Canary Validation Runbook (Docker Exec)

Use this runbook after a canary deploy to confirm the container is healthy.

## Checklist

- Service readiness (container running + health status not unhealthy)
- Key endpoint validation (`/api/version` with auth token)
- Required log patterns present (`Gas Town GUI starting`)
- Database connectivity (if applicable)

## Automated Validation Script

Run the validation from the host:

```bash
export GT_WEB_AUTH_TOKEN=...
deploy/canary-validate.sh --container gastown-canary --port 8081
```

Optional DB validation inside the container:

```bash
export GT_DB_CHECK_CMD='gt doctor db'
deploy/canary-validate.sh
```

## Failure Handling

- Validation failure exits non-zero.
- `deploy/canary-deploy.sh` treats this as a deploy failure and triggers rollback.
- Capture diagnostics:
  - `docker logs --tail 200 gastown-canary`
  - `docker inspect gastown-canary`

## Preconditions
- `deploy/canary-validate.sh` exists and is executable.
- The canary container (`gastown-canary`) is running and reachable on `8081`.
- `GT_WEB_AUTH_TOKEN` and `CANARY_PORT` are exported for API checks.

## Steps
1. Run `deploy/canary-validate.sh --container gastown-canary --port 8081` from the host to run the defined health checks.
2. Inspect `docker logs gastown-canary --tail 200` and look for any `ERROR` or repeated startup failure messages.
3. If validation fails, trigger `scripts/canary-command-retry.sh --command health --no-retry` to capture failure context and re-run after addressing the issue.
4. Report the validation status through beads (`bd update <id> --status=...`) and update the canary tag once the checks pass.

## Verification
- `curl -fsS http://localhost:8081/api/version`
- `deploy/canary-validate.sh --help`
- `scripts/canary-command-retry.sh --command health --no-retry`
