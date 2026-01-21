# Hook town fallback

- Updated hook lookups to check town beads when crew worktrees point at rig beads.
- `gt hook`/`gt mol status` now resolve hooked beads and progress for hq-* work.
- `gt prime` and session state detection now see hooked work across town/rig dbs.
- Tests: `go test ./internal/cmd/...`.
