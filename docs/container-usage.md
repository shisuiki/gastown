# Gas Town Container Usage Guide

**Last Updated:** 2026-01-24

## Overview

The Gas Town canary container runs a full agent infrastructure:
- **gt daemon** - Core service for agent coordination
- **Deacon** - Patrol mode agent (tmux session `hq-deacon`)
- **Web UI** - Browser interface on port 8080
- **Claude CLI** - AI assistant integration
- **Codex CLI** - Code assistance integration

## Quick Reference

| Action | Command |
|--------|---------|
| Deploy/redeploy | `GT_WEB_AUTH_TOKEN=<token> ./deploy/canary-deploy-full.sh` |
| Enter container | `docker exec -it gastown-canary bash` |
| View logs | `docker logs gastown-canary` |
| Check status | `docker exec gastown-canary gt status` |
| Stop container | `docker stop gastown-canary` |
| Remove container | `docker rm -f gastown-canary` |

## Deployment

### Initial Deployment

```bash
cd /home/shisui/work/gastown
export GT_WEB_AUTH_TOKEN="your-secret-token"
./deploy/canary-deploy-full.sh
```

The script will:
1. Build the Docker image from `Dockerfile.full`
2. Set up credential directories for Claude and Codex
3. Stop any existing container
4. Start a new container with full mode
5. Wait for health check to pass
6. Verify all components are running

### Redeployment

Same command as initial deployment. The script handles:
- Stopping existing container
- Building new image with latest code
- Automatic rollback if new container fails health check

### Environment Variables

Override defaults by setting before running:

```bash
export CONTAINER_NAME=gastown-canary-test
export CANARY_PORT=8082
export GT_ROOT=/path/to/gtruntime
export CLAUDE_CREDS_DIR=/path/to/claude/creds
export CODEX_CREDS_DIR=/path/to/codex/creds
export HTTP_PROXY=http://proxy:port
./deploy/canary-deploy-full.sh
```

## Container Operations

### Entering the Container

```bash
# Interactive shell
docker exec -it gastown-canary bash

# Run single command
docker exec gastown-canary gt status
```

### Viewing Logs

```bash
# Follow all logs
docker logs -f gastown-canary

# Last 100 lines
docker logs --tail 100 gastown-canary

# Daemon log inside container
docker exec gastown-canary cat ~/.gt/daemon.log
```

### Session Management

The container uses tmux for agent sessions:

```bash
# List all sessions
docker exec gastown-canary tmux list-sessions

# Attach to deacon session
docker exec -it gastown-canary tmux attach -t hq-deacon

# Detach from tmux: Ctrl+b, then d
```

### Service Status

```bash
# Full status
docker exec gastown-canary gt status

# Daemon only
docker exec gastown-canary gt daemon status

# Check deacon session exists
docker exec gastown-canary tmux has-session -t hq-deacon && echo "Running"
```

## Rollback

### Automatic Rollback

If the new container fails health check during deployment, the script automatically rolls back to the previous image (if one exists).

### Manual Rollback

1. Find the previous image:
   ```bash
   docker images | grep gastown
   ```

2. Stop current container:
   ```bash
   docker rm -f gastown-canary
   ```

3. Start with previous image:
   ```bash
   docker run -d \
       --name gastown-canary \
       --restart=always \
       -p 8081:8080 \
       -v /home/shisui/gt-canary:/gt \
       -v /home/shisui/.claude-canary:/home/gastown/.claude \
       -v /home/shisui/.codex-canary:/home/gastown/.codex \
       -e GT_WEB_AUTH_TOKEN="$GT_WEB_AUTH_TOKEN" \
       -e GT_WEB_ALLOW_REMOTE=1 \
       gastown:canary-full-<previous-sha> full
   ```

### Rollback from State File

The deploy script saves state to `/home/shisui/gt-canary/logs/canary-deploy.env`:

```bash
source /home/shisui/gt-canary/logs/canary-deploy.env
echo "Previous image: $PREVIOUS_IMAGE"
echo "Current image: $CURRENT_IMAGE"
```

## Credential Management

### Claude CLI

Credentials persist across container restarts via volume mount.

**Check authentication:**
```bash
docker exec gastown-canary claude --version
```

**Re-authenticate (if needed):**
```bash
docker exec -it gastown-canary claude login
```

Credentials stored at:
- Host: `/home/shisui/.claude-canary/.credentials.json`
- Container: `/home/gastown/.claude/.credentials.json`

### Codex CLI

**Check authentication:**
```bash
docker exec gastown-canary codex --version
```

**Re-authenticate (if needed):**
```bash
docker exec -it gastown-canary codex auth
```

Credentials stored at:
- Host: `/home/shisui/.codex-canary/auth.json`
- Container: `/home/gastown/.codex/auth.json`

## Proxy Configuration

API calls route through Clash proxy on host (port 7890):

```bash
# Verify proxy access from container
docker exec gastown-canary curl -x http://host.docker.internal:7890 https://api.anthropic.com/v1/models
```

Proxy environment variables are set automatically:
- `HTTP_PROXY=http://host.docker.internal:7890`
- `HTTPS_PROXY=http://host.docker.internal:7890`
- `NO_PROXY=localhost,127.0.0.1,host.docker.internal`

## Troubleshooting

### Container Won't Start

1. Check Docker logs:
   ```bash
   docker logs gastown-canary
   ```

2. Check image exists:
   ```bash
   docker images | grep gastown
   ```

3. Check disk space:
   ```bash
   df -h
   ```

4. Check mounts exist:
   ```bash
   ls -la /home/shisui/gt-canary
   ls -la /home/shisui/.claude-canary
   ```

### Health Check Failing

1. Check health status:
   ```bash
   docker inspect --format='{{.State.Health.Status}}' gastown-canary
   ```

2. Check health log:
   ```bash
   docker inspect --format='{{json .State.Health}}' gastown-canary | jq
   ```

3. Verify daemon:
   ```bash
   docker exec gastown-canary gt daemon status
   ```

4. Verify deacon:
   ```bash
   docker exec gastown-canary tmux has-session -t hq-deacon
   ```

### Deacon Not Starting

1. Check Claude CLI is available:
   ```bash
   docker exec gastown-canary which claude
   ```

2. Check credentials:
   ```bash
   docker exec gastown-canary ls -la /home/gastown/.claude/
   ```

3. Check deacon logs:
   ```bash
   docker exec gastown-canary tmux capture-pane -t hq-deacon -p
   ```

### Web UI Not Accessible

1. Check port binding:
   ```bash
   docker port gastown-canary
   ```

2. Check nginx status:
   ```bash
   sudo systemctl status nginx
   ```

3. Test directly:
   ```bash
   curl -I http://localhost:8081
   ```

### Proxy Issues

1. Verify Clash is running:
   ```bash
   curl -I http://localhost:7890
   ```

2. Test from container:
   ```bash
   docker exec gastown-canary curl -v -x http://host.docker.internal:7890 https://httpbin.org/ip
   ```

## Architecture

```
Host Machine
├── /home/shisui/work/gastown     # Source code
├── /home/shisui/gt-canary        # GTRuntime (mounted as /gt)
├── /home/shisui/.claude-canary   # Claude credentials
├── /home/shisui/.codex-canary    # Codex credentials
└── Clash proxy (port 7890)

gastown-canary Container
├── /gt                           # GTRuntime mount
├── /home/gastown/.claude         # Claude credentials mount
├── /home/gastown/.codex          # Codex credentials mount
├── /usr/local/bin/gt             # Gas Town binary
├── /usr/local/bin/bd             # Beads CLI
├── /usr/local/bin/claude         # Claude CLI (npm global)
├── /usr/local/bin/codex          # Codex CLI (npm global)
└── tmux sessions
    ├── hq-deacon                 # Deacon patrol
    ├── hq-witness-*              # Witness agents
    └── hq-refinery-*             # Refinery agents

Reverse Proxy
├── gt.ananthe.party  → localhost:8080 (production)
└── gt2.ananthe.party → localhost:8081 (canary)
```
