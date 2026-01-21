# Phase 2: Automation

- Added `scripts/canary-docker-exec.sh` with built-in command routing, container restart attempt, timeout enforcement, and rollback command parsing.
- Wired the helper into `.github/workflows/canary-deploy.yml`, running the health/config/migrate/smoke commands in order and firing the rollback job on failure.
- Verified the Go test suite still passes (`go test ./...`).
