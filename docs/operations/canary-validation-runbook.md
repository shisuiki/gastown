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
