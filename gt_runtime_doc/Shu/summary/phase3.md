# Phase 3: Failure handling

- Documented failure scenarios, retry policy, rollback procedures, and alerting behavior for Step 10 (`hq-sic13`).
- Added `scripts/canary-command-retry.sh` and `scripts/canary-alert.sh` to support retries with exponential backoff plus alert delivery, then wired them into `.github/workflows/canary-deploy.yml`.
- Confirmed the codebase still passes `go test ./...` after these updates.
