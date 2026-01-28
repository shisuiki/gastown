# Progress

- Cleared for backlog dispatch workflow fix (hq-37635).
- Added `gt deacon backlog-dispatch` to pair ready issues with idle polecats.
- Inserted backlog dispatch step into `mol-deacon-patrol` (version bump to 9).
- Tests: `go test ./internal/cmd/...` passes after agent bead fallback fix.
- Fixed `gt hook`/`gt prime` hook lookup to fall back to town beads from crew worktrees.
- Tests: `go test ./internal/cmd/...` passes.
- Created canary branch protections and env-config repo bootstrap; documented canary promotion and pairing manifest.
- Added canary deploy workflow, scripts, and host/rollback docs; recorded env-config pin.
- Added mayor → crew deploy request/status template and escalation rules.
- Documented canary promotion criteria, workflow, and regression handling.
- Audited branch policy sources; updated hooks/templates/docs to canary-first messaging.
