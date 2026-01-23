# Gas Town Project Status

**Last Updated:** 2026-01-24

## Repository Structure

### Two-Repository Model

| Repository | Location | Purpose |
|------------|----------|---------|
| **gastown** | `/home/shisui/work/gastown` | Source code, compiled to `gt` binary |
| **GTRuntime** | `/home/shisui/gt` | Runtime environment (TownRoot) |

The gastown repository also lives at `TerraNomadicCity/mayor/rig` in the broader project structure.

### Key Directories

**In gastown (source):**
- `cmd/` - CLI command implementations
- `internal/` - Core packages
- `deploy/` - Container deployment scripts
- `docs/` - Project documentation
- `gt_runtime_doc/` - Runtime documentation (has its own branches)

**In GTRuntime:**
- `.beads/` - Beads issue tracking (has its own branches)
- `mayor/` - Mayor agent configuration
- `deacon/` - Deacon patrol configuration
- `logs/` - Runtime logs
- `scripts/` - Operational scripts

## Branch Strategy

### gastown Repository

| Branch | Protection | Purpose |
|--------|------------|---------|
| **main** | Protected | Stable releases, requires PR with 1 approval |
| **canary** | Unprotected | Development branch, direct push allowed |
| **polecat/\*** | Unprotected | Agent work branches |

**Protection on main:**
- Requires 1 approving review
- Requires linear history (no merge commits)
- Dismisses stale reviews on new commits
- Enforced for administrators

**Development workflow:**
1. Work on `canary` branch
2. When stable, create PR from `canary` to `main`
3. Get approval and merge with rebase

### Special Branches

- `gt_runtime_doc/` folder has its own documentation branches
- `.beads/` in GTRuntime has its own sync branches
- `polecat/*` branches are created by agents for isolated work

## Deployments

### Production (`gt.ananthe.party`)

- Container: `gastown` (non-canary)
- Port: 8080 (internal) → 443 (external)
- Branch: `main`
- Mode: Web UI only

### Canary (`gt2.ananthe.party`)

- Container: `gastown-canary`
- Port: 8081 (internal) → 443 (external)
- Branch: `canary`
- Mode: Full (daemon + deacon + web UI)
- Deploy script: `deploy/canary-deploy-full.sh`

## Environment Configuration

### Required Environment Variables

| Variable | Description |
|----------|-------------|
| `GT_WEB_AUTH_TOKEN` | Authentication token for web UI |
| `GT_ROOT` | Path to GTRuntime (default: `/gt` in container) |

### Optional Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `CONTAINER_NAME` | `gastown-canary` | Docker container name |
| `CANARY_PORT` | `8081` | Host port for canary |
| `CLAUDE_CREDS_DIR` | `~/.claude-canary` | Claude credentials directory |
| `CODEX_CREDS_DIR` | `~/.codex-canary` | Codex credentials directory |
| `HTTP_PROXY` | `http://host.docker.internal:7890` | Proxy for API access |

## Credential Management

### Claude CLI

Credentials are persisted at:
- Host: `/home/shisui/.claude-canary/.credentials.json`
- Container: `/home/gastown/.claude/.credentials.json`

To re-authenticate:
```bash
docker exec -it gastown-canary claude login
```

### Codex CLI

Credentials are persisted at:
- Host: `/home/shisui/.codex-canary/auth.json`
- Container: `/home/gastown/.codex/auth.json`

To re-authenticate:
```bash
docker exec -it gastown-canary codex auth
```

## Proxy Configuration

API access (Claude/Codex) routes through Clash proxy on the host:
- Proxy: `http://host.docker.internal:7890`
- Server location: Hong Kong
- Configured via `HTTP_PROXY` and `HTTPS_PROXY` environment variables

## Daily Operations

### Cold-Start Testing

Daily automated test via `mol-canary-coldstart-test` formula:
- Triggers at midnight
- Stops and restarts canary container
- Verifies all components operational
- Results: `/home/shisui/gt/logs/coldstart-tests/`

### Manual Test

```bash
bd mol wisp mol-canary-coldstart-test
```

## Related Documentation

- [Cold-Start Procedure](../gt_runtime_doc/operations/coldstart-procedure.md)
- [Container Usage Guide](container-usage.md)
