# Roadmap

## Phase 0: Requirements
- Pull latest deploy changes and review `deploy/gastown-web.service` and any new token requirements.
- Identify required token name and usage in deployment flow.

## Phase 1: Implementation
- Generate a secure deploy token and configure service/env as required.
- Redeploy the web UI using the updated deployment flow.

## Phase 2: Validation
- Verify service is running and web UI is reachable.
- Confirm token-based auth/deploy flow is active (logs/config check).

## Acceptance criteria
- Deployment uses the new token requirement and service restarts cleanly.
- Token is generated and stored in the expected place without breaking startup.
