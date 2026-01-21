# Summary

- Added `gt deacon backlog-dispatch` to dispatch ready work to idle polecats.
- Inserted backlog dispatch step into `mol-deacon-patrol` (version bump to 9).
- Tests: `go test ./internal/cmd/...` failed due to existing bd type validation in `done_test.go`.
