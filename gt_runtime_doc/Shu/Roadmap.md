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

## Phase 4: Failure handling and recovery (Step 10)
- Document the failure scenarios (CI/build/test, Docker daemon, container startup, and test flakiness) plus the standard responses for operators.
- Implement retry policies with configurable attempt limits and exponential backoff per failure type, then wire these policies into the canary workflow.
- Formalize automated rollback triggers, a manual rollback runbook, and the alerting channels for when things break.

## Acceptance criteria
- Failure scenarios are documented end-to-end with clear mitigation and escalation guidance.
- The canary workflow enforces retry policies and exponential backoff, failing only after the configured budget is exhausted.
- Automated rollback and alert triggers surface context for reruns while the docs describe how to recover manually if automation stalls.
