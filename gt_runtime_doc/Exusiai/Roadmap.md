# Roadmap

## Phase 0: Requirements
- Inspect `gastown-sync.service` status, recent logs, and configuration.
- Check `~/laplace/gastown-src` repo state, remotes, and latest commit vs origin.

## Phase 1: Remediation
- If service is stopped or failing, restart and repair config/path issues.
- If repo is stale, fix sync script or permissions to allow updates.

## Phase 2: Validation
- Confirm service is active and running on interval.
- Confirm local repo matches remote HEAD or is up-to-date.

## Acceptance criteria
- `gastown-sync.service` is running without errors.
- `~/laplace/gastown-src` is up-to-date with origin.
