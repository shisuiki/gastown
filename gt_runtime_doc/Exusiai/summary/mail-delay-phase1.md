# Mail Delay Phase 1

Updated `gt mail check --inject` to defer reminders until tmux is idle,
and when busy schedule a delayed retry instead of dropping the reminder.
A runtime marker prevents multiple pending timers, and the marker is
cleared once the reminder is emitted or when unread mail is cleared.
