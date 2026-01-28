# CI/CD canary agent activity monitor

- Added canary agent activity data to CI/CD status using tmux session scan and beads metrics.
- Extended CI/CD UI with an agent activity card showing sessions, activity age, and task/mail/error counts.
- Tests: `go test ./internal/web/...`.
