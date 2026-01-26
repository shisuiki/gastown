# Mail System Audit Phase 1 Summary

## Changes
- Added `gt mail check --inject --nudge` and `--session` to deliver reminders via tmux nudge without blocking.
- Made inject scheduling identity-aware and per-session, so deferred retries target the correct inbox.
- Added timeouts for tmux idle checks to prevent hanging in inject mode.
- Added subject limiting in reminders to avoid oversized injections.

## Rationale
- Prevents blocked tmux panes and ensures deferred injections actually reach the right agent.
- Avoids silent failures when background checks run outside an agent directory.

## Notes
- Deferred injects now use `--nudge` and `--identity` to avoid running interactive commands in tmux panes.
