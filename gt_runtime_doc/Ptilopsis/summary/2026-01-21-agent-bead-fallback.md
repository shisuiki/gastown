# Summary

- Restored agent bead creation compatibility by retrying without type=agent when unsupported.
- Added fallback hook updates via description and hook parsing in done hook lookup.
- Tests: `go test ./internal/cmd/...` passes.
