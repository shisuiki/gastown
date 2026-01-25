# gastown-sync Audit Phase 1 Summary

## Findings
- `gastown-sync.service` was running but the source repo (`~/laplace/gastown-src`) was on `canary`.
- The sync script always pulls `origin/main`, causing `git pull` to error with divergent branches while the pipeline masked the failure (no `pipefail`).

## Changes
- Switched the source repo to `main` and fast-forwarded to `origin/main` (now at `621570ae`).
- Updated `/home/shisui/gt/scripts/gastown-sync.sh` to enable `pipefail` and to honor `GASTOWN_SYNC_BRANCH` (default `main`).
- Restarted `gastown-sync.service` to apply the updated script.

## Result
- Service is active and watch loop is running; repo is up-to-date on `main` so web content should track latest changes.
