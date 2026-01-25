# gastown-sync Audit Final Summary

## What changed
- Fixed the mismatch between sync script and repo branch by moving `~/laplace/gastown-src` to `main` and updating the sync script to handle pull errors correctly.
- `gastown-sync.service` was restarted and is running cleanly.

## Why
- The previous branch mismatch caused `git pull` to fail silently, preventing new web UI changes from landing.

## Follow-ups
- If you want to track another branch (e.g., `canary`), set `GASTOWN_SYNC_BRANCH` in the service environment and switch the repo to the same branch.
