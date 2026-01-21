# Canary deploy workflow

- Added canary deploy workflow (`.github/workflows/canary-deploy.yml`) running on self-hosted canary runners.
- Implemented `deploy/canary-deploy.sh` with Docker build/run, health checks, metadata logging, and rollback on failure.
- Added `deploy/canary-rollback.sh` and `docs/CANARY-DEPLOY.md` to document host setup, secrets, and rollback.
- Pinned env-config ref in `deploy/canary-manifest.yaml`.
