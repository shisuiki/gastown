# Mail Inject Idle Gate Final Summary

Mail injection now waits for two minutes of tmux inactivity before emitting
system-reminder output in `gt mail check --inject`. The gate is silent and
non-fatal, preserving the existing inject behavior while preventing mid-work
interruptions in active sessions.
