---
type: report
status: archived
owner: "unowned"
audience: dev
applies_to:
  repo: gastown
  branch: unknown
last_validated: "unknown"
source_of_truth: []
---

# Gas Town Work Dispatch Failure Modes Report

**Date:** 2026-01-17
**Context:** Attempted to sling 7 web UI work tickets to polecats; all failed to execute

---

## Executive Summary

Work dispatch via `gt sling` silently failed due to beads database fragmentation, agent bead creation failures, and mail routing issues. Polecats spawned but received no visible work, leading to idle "done" status despite pending tasks.

---

## Failure Modes Identified

### 1. Beads Database Fragmentation

**Symptom:** `gt show gt-ikx` returns "no issue found"

**Cause:** Two separate beads databases exist:
- `/home/shisui/gt/.beads/beads.db` (town-level, where mayor creates issues)
- `/home/shisui/gt/gastown/mayor/rig/.beads/beads.db` (rig-level, where polecats look)

Polecat worktrees have `.beads/redirect` pointing to `mayor/rig/.beads`, not town-level.

**Impact:** Work beads created by mayor invisible to polecats.

**Fix Stubs:**

*Policy/AGENTS.md:*
```markdown
## Beads Database Routing
- Mayor MUST create work beads from within gastown/ directory (not town root)
- Or use `--rig gastown` flag to ensure correct DB routing
- Verify with `bd info` before creating issues
```

*Core gt change:*
```go
// internal/cmd/sling.go - Add DB validation before sling
func validateBeadVisibility(beadID, targetRig string) error {
    // Check bead exists in target rig's beads DB
    // Return clear error if bead only exists in different DB
}
```

---

### 2. Agent Bead Creation Failures

**Symptom:** Warnings during sling:
```
could not set hook slot: issue not found
could not create agent bead: prefix mismatch
```

**Cause:** Agent bead IDs use format `gt-gastown-polecat-<name>` but DB prefix is `gt-`, causing prefix validation failures.

**Impact:** Polecats spawn without agent beads; `gt hook` returns "cannot determine agent identity"

**Fix Stubs:**

*Core gt change:*
```go
// internal/cmd/polecat.go - Fix agent bead ID generation
func createAgentBead(rig, name string) error {
    // Use simple prefix: gt-<name> instead of gt-<rig>-polecat-<name>
    // Or add --force to bypass prefix validation for agent beads
}
```

*Alternative - beads config:*
```yaml
# .beads/config.yaml
allow_prefixes:
  - gt-
  - gt-gastown-polecat-
  - gt-gastown-crew-
```

---

### 3. Mail Routing Failures

**Symptom:** Mail sent to `gastown/nux` but polecat inbox shows 0 messages

**Cause:** Mailbox address resolution fails silently. Polecat checks inbox at wrong path or mailbox doesn't exist.

**Impact:** Tasks sent via mail never visible to workers.

**Fix Stubs:**

*Core gt change:*
```go
// internal/mail/routing.go - Add delivery verification
func (m *Mailer) Send(to string, msg Message) error {
    // After send, verify message appears in recipient's inbox
    // Return error if delivery verification fails
    // Log: "Message %s delivered to %s (verified)" or "DELIVERY FAILED"
}
```

*Policy/AGENTS.md:*
```markdown
## Mail Delivery Verification
After sending critical task mail, verify delivery:
  gt mail inbox --identity <recipient>
If 0 messages, delivery failed - check mailbox path.
```

---

### 4. Silent Sling Failures

**Symptom:** `gt sling` reports success but work never executes

**Cause:** Sling process has multiple steps that can fail independently:
1. Spawn polecat ✓
2. Create agent bead ✗ (warning only)
3. Set hook slot ✗ (warning only)
4. Create convoy ✗ (warning only)
5. Send start prompt ✓

Non-fatal warnings don't stop the process, leaving polecat in broken state.

**Impact:** Mayor believes work is dispatched; polecats sit idle.

**Fix Stubs:**

*Core gt change:*
```go
// internal/cmd/sling.go - Fail fast on critical steps
func runSling(beadID, target string) error {
    // Make hook slot and agent bead creation FATAL errors
    // If these fail, don't send start prompt
    // Clean up: nuke the spawned polecat
    // Return clear error: "Sling failed: could not set hook"
}
```

*Add sling verification:*
```go
// After sling, verify:
// 1. Agent bead exists with correct hook_bead
// 2. Bead is visible from polecat's working directory
// 3. Polecat session is alive
func verifySling(polecatID, beadID string) error
```

---

### 5. Misleading "done" Status

**Symptom:** `gt rig status` shows polecats as "done" while they're actually stuck/idle

**Cause:** Status derived from empty `hook_bead` slot, not actual work completion.

**Impact:** Mayor cannot distinguish "completed work" from "never received work"

**Fix Stubs:**

*Core gt change:*
```go
// internal/cmd/rig.go - Improve status detection
func getPolecatStatus(p Polecat) string {
    if p.HookBead == "" && p.SessionAlive() {
        // Check: did this polecat ever have a hook?
        // Check: any completed work in last session?
        if noWorkHistory(p) {
            return "idle (no work received)"
        }
    }
    return "done"
}
```

---

## Recommended Immediate Mitigations

1. **Always create beads from rig directory:**
   ```bash
   cd gastown && bd create "title" ...
   ```

2. **Verify bead visibility before sling:**
   ```bash
   bd show <bead-id>  # from polecat worktree path
   ```

3. **Use mail with delivery check:**
   ```bash
   gt mail send <target> -s "..." -m "..."
   gt mail inbox --identity <target>  # verify delivery
   ```

4. **Check polecat state after sling:**
   ```bash
   gt hook show <rig>/<polecat>  # should show hooked bead
   ```

---

## Architecture Consideration

The fundamental issue is **distributed state without consistency guarantees**. Work dispatch touches:
- Beads DB (possibly wrong one)
- Agent beads (may fail to create)
- Mail system (may fail to route)
- Tmux sessions (may not receive prompts)

Consider: **Transactional sling** that either fully succeeds or fully rolls back.

---

## Directory Structure Analysis

### Overview

```
/home/shisui/gt/                    # "Town" root
├── .beads/beads.db                 # Town-level beads (38 issues) - WRONG DB
├── gastown-src/                    # Source code (Go project)
│   ├── .git → github.com/shisuiki/gastown.git
│   └── internal/web/              # Web UI code lives here
├── gastown/                        # Rig directory
│   ├── .beads/redirect → mayor/rig/.beads
│   ├── .repo.git                  # Local bare repo (polecats clone from here)
│   ├── config.json                # local_repo: gastown-src
│   ├── gt                         # Rig's gt binary (OUTDATED)
│   ├── mayor/rig/.beads/beads.db  # Rig-level beads (2672 issues) - CORRECT DB
│   ├── polecats/
│   │   └── <name>/gastown/        # Polecat worktrees
│   └── gastown/                   # Nested rig (why??)
│       ├── .beads/redirect → mayor/rig/.beads
│       └── config.json            # git_url: file:///.repo.git
└── FAILURE_MODES_REPORT.md        # This file
```

### Key Findings

#### 1. Two Beads Databases (Root Cause of Dispatch Failures)

| Database | Location | Issue Count | Who Uses It |
|----------|----------|-------------|-------------|
| Town-level | `/home/shisui/gt/.beads/beads.db` | 38 | Mayor (from gastown-src) |
| Rig-level | `/home/shisui/gt/gastown/mayor/rig/.beads/beads.db` | 2672 | Polecats, Witness, Refinery |

**Problem:** When mayor runs `bd create` from `gastown-src`, issues go to town-level DB.
Polecats can't see them because their `.beads/redirect` points to rig-level DB.

#### 2. Three Git States (Deployment Sync Issue)

| Location | HEAD | Remote |
|----------|------|--------|
| `gastown-src` | `550d8259` (latest) | github.com/shisuiki/gastown.git |
| `gastown/.repo.git` | `51509115` (behind) | - |
| `gastown/gastown` | `c3cb631` (very behind) | file:///.repo.git |

**Problem:** Changes committed to gastown-src don't automatically propagate to:
1. The rig's bare repo (`.repo.git`)
2. Polecat worktrees

**Deployment Flow (appears intended):**
```
gastown-src → push → github → fetch → .repo.git → polecat worktrees
```

But this flow requires manual sync or a deploy script.

#### 3. Two gt Binaries

| Binary | Modified | MD5 |
|--------|----------|-----|
| `/home/shisui/go/bin/gt` | 13:19 (rebuilt) | f858d60f... |
| `/home/shisui/gt/gastown/gt` | 04:31 (old) | 309ea4b7... |

**Problem:** Polecats may use the rig's outdated binary depending on PATH.

#### 4. Nested gastown/gastown Structure

The nested `gastown/gastown/` appears to be an artifact of rig initialization that creates a worktree copy of the source repo. This is intentional but confusing:

- `gastown/` = rig infrastructure (config, polecats, witness, etc.)
- `gastown/gastown/` = working copy of source code for polecats

### Recommended Directory Structure Improvements

1. **Consolidate to single beads DB:**
   - Remove town-level `.beads/` or add redirect to rig-level
   - All work creation should target rig-level DB

2. **Add deploy script to sync git repos:**
   ```bash
   # After committing to gastown-src:
   cd gastown-src && git push origin main
   cd ../gastown/.repo.git && git fetch origin && git reset --hard origin/main
   # Update polecat worktrees
   ```

3. **Standardize gt binary:**
   - Remove `gastown/gt` or symlink to `/home/shisui/go/bin/gt`
   - Ensure PATH prioritizes installed binary

4. **Document the nested structure:**
   - Rename `gastown/gastown/` to `gastown/work/` or `gastown/repo/` for clarity
   - Or flatten if the nesting is unnecessary

---

## Handoff Summary

**For next session:**

1. **Web UI changes are committed** to `gastown-src` at `550d8259` but need sync to rig
2. **gt binary is rebuilt** at `/home/shisui/go/bin/gt` and GUI is running with new code
3. **Work dispatch is broken** - don't use `gt sling` until DB consolidation is done
4. **Manual workaround:** Create beads from within `gastown/` directory, not `gastown-src`

**Immediate todos for next revision cycle:**
- [ ] Sync `gastown-src` commits to `.repo.git`
- [ ] Consolidate beads DBs (redirect town-level to rig-level)
- [ ] Fix agent bead prefix mismatch
- [ ] Add sling pre-flight validation

---

## Next Steps

1. File these as tracked issues for gt core improvements
2. Update AGENTS.md with workaround policies
3. Add pre-flight checks to sling command
4. Consider beads DB consolidation (single source of truth)
5. Implement auto-deploy to sync gastown-src → .repo.git → polecats
