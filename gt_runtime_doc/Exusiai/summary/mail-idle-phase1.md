# Mail Inject Idle Gate Phase 1

Implemented a tmux idle gate for `gt mail check --inject` so reminders only emit
after two minutes of session inactivity. The logic checks the current tmux session
activity timestamp (falling back to last-attached) and silently skips injection
when the session is active or when idle state cannot be determined. This prevents
mail popups from interrupting ongoing agent work while keeping non-inject
behavior unchanged.
