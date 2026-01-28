# Phase 1 Summary - Dog Session-GC Timeout Fix

## Scope
- Reduce `mol-session-gc` dog runtime to avoid 10-minute dog timeout.
- Ensure cleanup operations are fast and not dependent on agent behavior alone.

## Changes
- Added a `--profile cleanup` mode to `gt doctor` that only runs cleanup checks (orphan sessions, zombie sessions, orphan processes, wisp GC).
- Auto-selects cleanup profile when running inside dog contexts (detects `GT_ROLE`, `BD_ACTOR`, `GT_ROLE_HOME`, or dog paths under `deacon/dogs`).
- Updated `mol-session-gc.formula.toml` to explicitly use `--profile cleanup` for preview, fix, and verify steps.
- Documented cleanup profile usage in `docs/reference.md`.

## Tests
- `go test ./internal/cmd -run TestDoctor`
- `go test ./internal/doctor`

## Notes / Risks
- Full `gt doctor` behavior is unchanged for humans; cleanup profile is explicit or dog-context only.
- If dog sessions run outside dog paths and without env markers, the formula’s explicit profile flag still enforces fast checks.
