# Progress

- Added prompt partials and reordered role templates with critical rules and Do-Not-Do sections.
- Added AgentName to role data and updated crew/polecat templates for naming clarity.
- Aligned system prompts with role boundaries and commit-granularity guidance.
- Added --confirm guardrails to high-risk gt commands and documented new requirement.
- Closed prompt optimization issues: hq-zqyl, hq-5abf, hq-gpxx, hq-dy59, hq-u9lw.
- Ran `go test ./internal/templates`.
- Rebuilt `gt` from repo to include quoting fix; `gt mayor start` now succeeds.
- Fixed auto-sync chain: updated post-merge hook to restart user-level web service, added PATH/HOME to sync service, enabled linger, added user logrotate timer for sync log, restarted web service under systemd.

## 2026-01-20: Triage - Orphaned Polecat Branches + Missing MR Beads

### Findings

**22 orphaned polecat branches** exist on origin, but:
- 0 merge-request beads in the system
- Refinery queue is empty
- Most branches have 1 unmerged commit

**Root cause**: Polecats are not calling `gt done` before exiting.

The `gt done` command is responsible for:
1. Pushing the branch to origin
2. Creating the MR bead (`type: merge-request`)
3. Notifying the Witness

Without `gt done`, no MR bead is created. The Witness finds "orphan" polecats during patrol and sends MERGE_READY to Refinery, but with "Issue: none" because there's no MR bead to reference.

**Evidence**:
- MERGE_READY messages in Refinery inbox say "Issue: none"
- Source issues (e.g., hq-ge23) still show status=HOOKED
- Branches have commits not on main

### Breakdown by Branch State

**Already merged (0 unmerged commits)** - safe to delete:
- `origin/polecat/chrome/hq-d2p1@mkm5p0ym` (hq-d2p1 is CLOSED)

**Unmerged work (1+ commits)** - need MR beads:
- Most branches (hq-ge23, te-3aq, te-3fr, furiosa, etc.)

### Recommended Recovery

1. **For branches with unmerged work**: Create MR beads manually
2. **For branches already merged**: Delete the orphaned branches
3. **For source issues still HOOKED**: Unhook or close based on merge state

See hq-uog2 (assigned to shiny) for MR bead closure after merge.

## 2026-01-20: hq-uog2 - Fix Refinery MR Bead Closure

**Problem**: After refinery merges branches, MR beads remain OPEN because:
1. Claude refinery agent does manual git merges
2. Agent is supposed to run `bd close` manually per formula instructions
3. Error-prone: steps get skipped, MR bead IDs not tracked properly

**Solution**: Added `gt mq merged` command (commit 82a232de) that atomically handles ALL post-merge cleanup:
- Closes MR bead with merge commit SHA
- Closes source issue with merge commit SHA
- Sends MERGED notification to Witness
- Optionally notifies Mayor (--notify-mayor)
- Optionally deletes source branch (--delete-branch)

**Usage**:
```bash
gt mq merged <mr-id-or-branch> --commit $(git rev-parse HEAD) --delete-branch --notify-mayor
```

Updated refinery patrol formula to use this command instead of manual `bd close` calls.
