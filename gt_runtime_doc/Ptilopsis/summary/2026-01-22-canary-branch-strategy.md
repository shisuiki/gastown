# Canary branch strategy and env-config

- Created `canary` branch in shisuiki/gastown and applied branch protection with CI + PR review requirements.
- Bootstrapped shisuiki/env-config repo with canary/production configs and a minimal CI check.
- Added `deploy/canary-manifest.yaml` and documented promotion criteria + version pairing in README.
- Branch push restrictions are unavailable for user-owned repos; protections rely on PR reviews + CI.
