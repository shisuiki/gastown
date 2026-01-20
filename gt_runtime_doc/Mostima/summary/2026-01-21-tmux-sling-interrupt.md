# Incident Report: Repeated "Work slung" tmux injections with empty hook

Date: 2026-01-21
Severity: P2 (user reported)

Summary
- Multiple tmux sessions intermittently receive the prompt: "Work slung: <id>. Start working on it now - run `gt hook` to see the hook, then begin."
- The injected prompt does not always include a trailing Enter, and `gt hook` shows no hooked work afterward.

Impact
- Active tmux sessions are interrupted and can appear hung waiting for input.
- Operators lose confidence in hook state and may stop or restart sessions to recover.

Detection
- Reported by user after repeated identical prompts in tmux sessions.

Timeline (investigation)
- 2026-01-21 01:10: Gathered code references for the prompt string.
- 2026-01-21 01:12: Audited systemd/cron processes and running `gt` processes.
- 2026-01-21 01:15: Checked tmux session environments and events log.
- 2026-01-21 02:05: Captured crew tmux panes and located repeated "gt-wisp-xyz" prompts; checked beads DBs.

Findings
- The exact prompt text is generated only by `gt sling` via `injectStartPrompt` in `internal/cmd/sling_helpers.go`.
- No systemd timer or cron job runs `gt sling` directly. Crontab only runs:
  - `gt dog call boot`
  - `gt nudge deacon 'patrol'`
- User systemd services include `gastown-sync.service` (source pull watcher) and `gastown-gui` (web).
- `gt` cron logs exist (`logs/boot-cron.log`, `logs/deacon-cron.log`) but show only repeated failures for `gt dog call` / `gt nudge` due to missing `bd` in PATH.
- `gt daemon` runs with `TMUX` and `TMUX_PANE` inherited from a tmux session (`TMUX_PANE=%3`, session `gt-boot`), which means any self-targeted `gt sling` run in that environment would inject into that pane.
- tmux sessions show normal `GT_*` env; no `BEADS_DIR` set in sessions.
- Event log shows slings from mayor/deacon only; no unaccounted repeated sling events to crew sessions.
- `gt-wisp-xyz` appears repeatedly in `gt-TerraNomadicCity-crew-Myrtle` scrollback; no bead with that ID exists in town or crew beads DBs.

Likely Root Cause (hypothesis)
- The prompt is being injected by repeated `gt sling` calls (the only source of that string).
- These calls appear automated or looped, not interactive, and may be running in a process that inherited tmux env.
- The repeated `gt-wisp-xyz` suggests a placeholder or invalid bead ID; `gt sling` can accept bead-like IDs even if `verifyBeadExists` fails, and a `bd update` failure might not halt the prompt if bd exits 0 with stderr only.
- The empty hook after the prompt is consistent with slinging a town-level bead while `gt hook` reads only the local rig beads DB, or with a misrouted beads DB during sling.

Contributing Factors
- No centralized logging of `gt sling` nudges (bead ID, target pane, caller) for correlation.
- `tmux send-keys` errors on the Enter key are not surfaced to the receiving session.
- `gt hook` uses the nearest `.beads` (rig-level) and does not check town-level beads for crew by default.

Mitigation / Current Status
- No changes applied during investigation.

Recommended Next Steps
1. Add logging for `gt sling` nudges (bead ID, target agent, target pane, caller) to `town.log` or `.events.jsonl`.
2. Clear `TMUX`/`TMUX_PANE` in daemon/systemd environments to prevent accidental self-targeted nudges.
3. Update `gt hook` to consult town-level beads when the slung bead is an `hq-*` wisp or town-level task.
4. Capture the exact bead ID and timestamp the next occurrence to correlate against `.events.jsonl`.
5. Audit any wrappers or aliases that invoke `gt sling` with placeholder IDs; confirm `bd update` behavior on non-existent IDs.
