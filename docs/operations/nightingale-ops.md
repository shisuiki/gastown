---
type: runbook
status: active
owner: "unowned"
audience: ops
applies_to:
  repo: gastown
  branch: canary
last_validated: "unknown"
ttl_days: 30
next_review: "unknown"
source_of_truth:
  - "scripts/nightingale-trigger.sh"
  - ".github/workflows/nightingale-trigger.yml"
---

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

GitHub Actions can call `scripts/nightingale-trigger.sh` (for example from a workflow such as `.github/workflows/nightingale-trigger.yml`) on `self-hosted` runners labeled `nightingale`.

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

## Cold-Start Test Protocol

When running cold-start tests on gastown-canary:

### Communication Channel

The containerized mayor replies to `nightingale/Nightingale` (this crew's inbox).

1. **Send probe:** `docker exec gastown-canary gt mail send mayor/ -s "COLDSTART_PROBE: ..."`
2. **Mayor processes:** Canary mayor sees probe, gathers status, replies
3. **Receive response:** Check `gt mail inbox` for `COLDSTART_ASSESSMENT` subject

### Context Injection (if needed)

If the canary mayor needs context about the test:
```bash
# Nudge mayor with context
docker exec gastown-canary gt nudge mayor "Nightingale CI/CD test in progress. Reply to nightingale/Nightingale."
```

### Verifying Mayor Responsiveness

```bash
# Check if mayor session is active
docker exec gastown-canary tmux has-session -t hq-mayor

# If not active, start it
docker exec gastown-canary gt mayor start

# Attach to mayor for manual interaction
docker exec -it gastown-canary gt mayor attach
```

## Notes

- If the host rig is unavailable, the workflow should fail and be retried.
- Beads act as the durable audit trail for CI/CD actions.
- Canary mayor is configured to know it's containerized and how to communicate with Nightingale.

## Preconditions
- The `nightingale` rig and crew workspaces exist (`gt rig list`, `gt crew list`).
- The self-hosted runner has label `nightingale` and can reach this repo.
- `scripts/nightingale-trigger.sh` is up to date and owned by the team.

## Steps
1. Trigger the workflow via `gh workflow run nightingale-trigger.yml --ref main` or the dispatch call shown above.
2. Inspect the bead created by the script (`bd show <auto-created bead>`) and ensure it references the intended canary job.
3. Follow `docs/operations/canary-docker-exec-workflow.md` when Nightingale receives work, linking status updates through `docs/CANARY-DEPLOY.md`.
4. If Nightingale fails, nudge the rig owner with the alert in `docs/operations/canary-failure-handling.md` and archive related logs.

## Verification
- `gt rig list | grep nightingale`
- `gt crew list --rig nightingale`
- `scripts/nightingale-trigger.sh --help`
- `gt status -s` (ensure no pending work blocking Nightingale)
