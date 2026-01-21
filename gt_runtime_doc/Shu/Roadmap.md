# Roadmap

## Phase 1: Lock in the requirements
- Record the canary Docker-exec workflow so future phases codify the command library, error paths, and rollback triggers.
- Describe the standard commands (health, config reload, migration, smoke test) and the expectations for the deploy workflow.

## Phase 2: Build the automation
- Create a reusable `scripts/canary-docker-exec.sh` wrapper that enforces timeouts, container health checks, and exit handling for the standard commands.
- Add GitHub workflow `canary-deploy.yml` to run each step, call rollbacks on failures, and run the wrapper from a controlled environment.

## Phase 3: Validate and wrap up
- Run the necessary tests (Go unit tests, linting if applicable) to keep the repo healthy.
- Capture phase summaries in `gt_runtime_doc/Shu/summary` and finalize the status in `Progress.md`.

## Acceptance criteria
- There is an operations reference for Step 5 tied to the Docker exec trigger mechanism.
- The wrapper script runs standardized commands, handles container restarts, enforces timeouts, and returns clear exit codes.
- The new workflow runs each command in order and triggers the rollback job automatically when a failure occurs.
