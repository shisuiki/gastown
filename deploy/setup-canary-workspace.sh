#!/usr/bin/env bash
#
# setup-canary-workspace.sh - Initialize the canary workspace for unified CI/CD
#
# This script creates a git worktree at /home/shisui/gt-canary that tracks
# the 'canary' branch of the GTRuntime repo. This allows the canary container
# to use canary versions of formulas and runtime docs while production uses master.
#
# Usage:
#   ./setup-canary-workspace.sh [--force]
#
# Options:
#   --force    Remove existing canary workspace and recreate
#

set -euo pipefail

CANARY_ROOT=${CANARY_ROOT:-/home/shisui/gt-canary}
PRODUCTION_ROOT=${PRODUCTION_ROOT:-/home/shisui/gt}
CANARY_BRANCH=${CANARY_BRANCH:-canary}

log() {
  printf "[%s] %s\n" "$(date -u +%Y-%m-%dT%H:%M:%SZ)" "$*"
}

fail() {
  log "ERROR: $*"
  exit 1
}

FORCE=0
if [ "${1:-}" = "--force" ]; then
  FORCE=1
fi

# Validate production root exists and is a git repo
if [ ! -d "$PRODUCTION_ROOT/.git" ] && [ ! -f "$PRODUCTION_ROOT/.git" ]; then
  fail "Production root is not a git repository: $PRODUCTION_ROOT"
fi

cd "$PRODUCTION_ROOT"

# Check if canary branch exists
if ! git show-ref --verify --quiet "refs/heads/$CANARY_BRANCH" && \
   ! git show-ref --verify --quiet "refs/remotes/origin/$CANARY_BRANCH"; then
  log "Creating canary branch from current HEAD..."
  git checkout -b "$CANARY_BRANCH"
  git push -u origin "$CANARY_BRANCH"
  git checkout master
  log "Canary branch created and pushed"
fi

# Handle existing canary workspace
if [ -d "$CANARY_ROOT" ]; then
  if [ "$FORCE" -eq 1 ]; then
    log "Removing existing canary workspace (--force specified)"
    # Check if it's a worktree
    if git worktree list | grep -q "$CANARY_ROOT"; then
      git worktree remove "$CANARY_ROOT" --force
    else
      rm -rf "$CANARY_ROOT"
    fi
  else
    log "Canary workspace already exists at $CANARY_ROOT"
    log "Use --force to recreate, or manually verify the workspace"
    exit 0
  fi
fi

# Create the canary worktree
log "Creating canary worktree at $CANARY_ROOT"
git worktree add "$CANARY_ROOT" "$CANARY_BRANCH"

# Verify the workspace
log "Verifying canary workspace..."

if [ ! -d "$CANARY_ROOT/.beads/formulas" ]; then
  fail "Canary workspace missing .beads/formulas"
fi

if [ ! -d "$CANARY_ROOT/gt_runtime_doc" ]; then
  fail "Canary workspace missing gt_runtime_doc"
fi

CANARY_SHA=$(git -C "$CANARY_ROOT" rev-parse HEAD)
CANARY_BRANCH_ACTUAL=$(git -C "$CANARY_ROOT" branch --show-current)

log "=== Canary Workspace Setup Complete ==="
log "Location: $CANARY_ROOT"
log "Branch: $CANARY_BRANCH_ACTUAL"
log "SHA: $CANARY_SHA"
log ""
log "To sync canary workspace with remote:"
log "  cd $CANARY_ROOT && git fetch origin $CANARY_BRANCH && git reset --hard origin/$CANARY_BRANCH"
log ""
log "To push changes from canary to production:"
log "  cd $PRODUCTION_ROOT && git checkout master && git merge $CANARY_BRANCH"
