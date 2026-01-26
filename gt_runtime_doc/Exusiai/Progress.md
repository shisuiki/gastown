# Progress

- Added mail check `--nudge`/`--session` flags, per-session inject markers, identity-aware scheduling, and safer tmux idle checks.
- Implemented mail reminder builder with subject limiting for inject output.
- Added deferred mail notifications when recipients are busy (schedule inject retry).
- Documented mail delivery + inject behavior in mail protocol design doc.
- Cleaned inject markers for idle/no-unread using detected session fallback.
