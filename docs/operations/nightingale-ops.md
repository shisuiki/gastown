# Nightingale CI/CD Ops

Nightingale is the host-rig crew used for CI/CD operations. It runs on a
CI/CD-dedicated rig (not the canary container) and is triggered by GitHub
Actions via a self-hosted runner.

## Host Rig Requirements

- Rig name: `nightingale`
- Workspace root: `/home/shisui/gt/nightingale`
- Crew workspace: `/home/shisui/gt/nightingale/crew/Nightingale`
- Self-hosted GitHub Actions runner with label `nightingale`
- Docker CLI access for canary workflows

## Setup Checklist

1. Create the rig (once):
   ```bash
   gt rig add nightingale https://github.com/shisuiki/gastown.git
   ```
2. Create the crew workspace:
   ```bash
   gt crew add Nightingale --rig nightingale
   ```
3. Ensure the runner can execute `gt` and `bd` in PATH.

## Trigger Mechanism

GitHub Actions uses `.github/workflows/nightingale-trigger.yml` which runs on
`self-hosted` runners and calls `scripts/nightingale-trigger.sh`.

The script:
- Creates a bead describing the CI/CD trigger
- Slings it to `nightingale/crew/Nightingale`

### Manual Trigger

```bash
# GitHub Actions manual dispatch
gh workflow run nightingale-trigger.yml --ref main

# repository_dispatch (alternate)
gh api repos/shisuiki/gastown/dispatches \\
  -f event_type=nightingale-trigger \\
  -f client_payload[reason]=manual
```

## Hook Behavior

When Nightingale receives a hook, it should:
1. Read the bead description for the trigger context.
2. Follow `docs/operations/canary-docker-exec-workflow.md`.
3. Reference `docs/CANARY-DEPLOY.md` and `docs/MAYOR-CREW-DEPLOY.md` for status updates.

## Notes

- If the host rig is unavailable, the workflow should fail and be retried.
- Beads act as the durable audit trail for CI/CD actions.
