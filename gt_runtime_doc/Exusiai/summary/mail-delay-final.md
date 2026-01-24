# Mail Delay Final Summary

Mail injection now uses a delayed retry loop: if a session is active,
`gt mail check --inject` schedules a retry after 2 minutes (via tmux
run-shell) and records a runtime marker to avoid duplicate timers.
Once the session is idle, the reminder is injected and the marker is
cleared, ensuring the reminder is delayed—not lost.
