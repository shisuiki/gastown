---
type: evergreen
status: active
owner: "unowned"
audience: dev
applies_to:
  repo: gastown
  branch: main
last_validated: "unknown"
source_of_truth: []
---

# Gas Town Web UI Deployment

This document describes how the Gas Town Web UI is deployed and auto-updated.

## Architecture Overview

```
GitHub (shisuiki/gastown)
        ↓ (gastown-sync.service checks every 60s)
~/laplace/gastown-src (local source)
        ↓ git pull (triggers post-merge hook → go build)
~/go/bin/gt (binary with embedded webui)
        ↓ (sync auto-restarts web UI service)
http://localhost:8080 (Web UI)
```

## Components

### 1. Source Repository

- **Location**: `~/laplace/gastown-src`
- **Remote**: `https://github.com/shisuiki/gastown.git`
- **Git Hooks**:
  - `post-commit`: Auto-builds gt after local commits
  - `post-merge`: Auto-builds gt and triggers WebUI redeploy after pull

### 2. Systemd Services

This deployment uses one system-level service for the GUI and a user-level
service for sync. The legacy user-level `gastown-web.service` may still exist
but is typically disabled when `gastown-gui.service` is active.

#### gastown-gui.service (system-level)
Runs the Web UI server.

```ini
[Unit]
Description=Gas Town Web GUI
After=network.target

[Service]
Type=simple
User=shisui
WorkingDirectory=/home/shisui/gt
ExecStartPre=/home/shisui/gt/scripts/gastown-web-guard.sh
ExecStart=/home/shisui/go/bin/gt gui --port 8080
Restart=always
RestartSec=5
Environment=HOME=/home/shisui
Environment=PATH=/home/shisui/go/bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin
Environment=GASTOWN_WEB_PORT=8080
Environment=GASTOWN_SRC=/home/shisui/laplace/gastown-src
Environment=GT_WEB_POST_SAVE_HOOK=/home/shisui/gt/scripts/webui-git-sync.sh
Environment=GT_WEB_GIT_FAILOVER_TARGET=TerraNomadicCity
Environment=GT_WEB_GIT_AUTHOR_NAME=Gastown WebUI
Environment=GT_WEB_GIT_AUTHOR_EMAIL=shisuiki@users.noreply.github.com

[Install]
WantedBy=multi-user.target
```

#### gastown-sync.service (user-level)
Watches for upstream changes and auto-syncs.

```ini
[Unit]
Description=Gas Town Source Sync Watcher
After=network.target

[Service]
Type=simple
ExecStart=/home/shisui/gt/scripts/gastown-sync.sh watch
Restart=on-failure
RestartSec=10
Environment=GASTOWN_SRC=/home/shisui/laplace/gastown-src
Environment=GASTOWN_SYNC_INTERVAL=60

[Install]
WantedBy=default.target
```

### 3. Sync Script

`~/gt/scripts/gastown-sync.sh` handles the auto-update logic:

```bash
# Check if source is behind remote
./gastown-sync.sh check

# Manually sync (pull + rebuild + restart web)
./gastown-sync.sh sync

# Run as daemon (checks every 60s)
./gastown-sync.sh watch

# Stop watch daemon
./gastown-sync.sh stop

# Check daemon status
./gastown-sync.sh status
```

### 4. WebUI Git Sync Hook

The WebUI can run git sync after saving Config/Prompts. Configure these env vars
in `gastown-gui.service` if you want a custom hook:

- `GT_WEB_POST_SAVE_HOOK`: Optional script path to run after save. It receives:
  - `GT_WEB_GIT_REPO_ROOT`
  - `GT_WEB_GIT_COMMIT_MSG`
  - `GT_WEB_GIT_ACTION`
  - `GT_WEB_GIT_PATHS` (comma-separated)
- `GT_WEB_GIT_REPO_ROOT`: Override git root used for WebUI save-time commits.
- `GT_WEB_GIT_FAILOVER_TARGET`: Optional sling target for auto-recovery.
- `GT_WEB_GIT_AUTHOR_NAME` / `GT_WEB_GIT_AUTHOR_EMAIL`: Optional git author identity for commits.
- `GT_WEB_GIT_COMMITTER_NAME` / `GT_WEB_GIT_COMMITTER_EMAIL`: Optional committer identity (defaults to author if omitted).

If the hook is not set, the WebUI runs `git add`, `git commit`, and `git push`
directly. On failure it creates a bead and slings it to the configured target.

### 4. Webhook Server (Optional)

`~/gt/scripts/gastown-webhook.py` provides a webhook endpoint for GitHub Actions:

```bash
# Start webhook server on port 9876
GASTOWN_WEBHOOK_TOKEN=your_secret python3 ~/gt/scripts/gastown-webhook.py

# Trigger from GitHub Actions:
curl -X POST "http://your-server:9876/sync?token=your_secret"
```

## Installation

### 1. Install Systemd Services

```bash
# Copy service files (user-level sync)
mkdir -p ~/.config/systemd/user
cp deploy/gastown-sync.service ~/.config/systemd/user/

# Reload and enable (user-level sync)
systemctl --user daemon-reload
systemctl --user enable --now gastown-sync.service

# Enable linger for services to run without login
loginctl enable-linger $USER
```

System-level GUI service (managed in `/etc/systemd/system`):

```bash
sudo systemctl daemon-reload
sudo systemctl enable --now gastown-gui.service
```

### 2. Install Sync Script

```bash
mkdir -p ~/gt/scripts
cp deploy/gastown-sync.sh ~/gt/scripts/
chmod +x ~/gt/scripts/gastown-sync.sh
```

### 3. Configure Git Hooks (in gastown-src)

```bash
cd ~/laplace/gastown-src

# post-commit hook
cat > .git/hooks/post-commit << 'EOF'
#!/bin/bash
cd "$(git rev-parse --show-toplevel)"
echo "[post-commit] Building gt..."
go build -o ~/go/bin/gt ./cmd/gt 2>&1
[ $? -eq 0 ] && echo "[post-commit] gt built successfully" || echo "[post-commit] build failed"
EOF
chmod +x .git/hooks/post-commit

# post-merge hook
cat > .git/hooks/post-merge << 'EOF'
#!/bin/bash
cd "$(git rev-parse --show-toplevel)"
echo "[post-merge] Building gt..."
go build -o ~/go/bin/gt ./cmd/gt 2>&1
if [ $? -eq 0 ]; then
    echo "[post-merge] gt built successfully"
    if systemctl --user is-active gastown-web.service &>/dev/null; then
        echo "[post-merge] Restarting web service..."
        systemctl --user restart gastown-web.service
    fi
else
    echo "[post-merge] build failed"
fi
EOF
chmod +x .git/hooks/post-merge
```

## Management Commands

```bash
# View service status
systemctl --user status gastown-web gastown-sync

# Restart services
systemctl --user restart gastown-web
systemctl --user restart gastown-sync

# Stop services
systemctl --user stop gastown-web gastown-sync

# View logs (real-time)
journalctl --user -u gastown-web -f
journalctl --user -u gastown-sync -f

# View sync log file
tail -f ~/gt/logs/gastown-sync.log

# Manual sync
~/gt/scripts/gastown-sync.sh sync

# Check if behind remote
~/gt/scripts/gastown-sync.sh check
```

## Auto-Update Flow

1. Code is pushed to GitHub (`shisuiki/gastown`)
2. `gastown-sync.service` checks remote every 60 seconds
3. When new commits detected, runs `git pull` in `~/laplace/gastown-src`
4. `post-merge` hook triggers `go build -o ~/go/bin/gt`
5. Sync script restarts `gastown-web.service`
6. Web UI now serves the updated code

## Troubleshooting

### Service won't start

```bash
# Check logs
journalctl --user -u gastown-web -n 50

# Common issues:
# - Port 8080 in use: kill other process or change port
# - bd not found: ensure PATH includes ~/go/bin
# - Not in workspace: ensure WorkingDirectory is set to ~/gt
```

### Sync not working

```bash
# Check sync service logs
journalctl --user -u gastown-sync -f

# Manual check
cd ~/laplace/gastown-src
git fetch origin main
git rev-list --count HEAD..origin/main  # Shows commits behind
```

### Web UI shows old code

```bash
# Check gt binary timestamp
stat ~/go/bin/gt | grep Modify

# Check src commit
cd ~/laplace/gastown-src && git log --oneline -1

# Force rebuild and restart
cd ~/laplace/gastown-src
go build -o ~/go/bin/gt ./cmd/gt
systemctl --user restart gastown-web
```

## Scope
- Scope description pending.
