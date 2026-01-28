# Roadmap

## Phase 0: Discovery
- Locate mol-session-gc formula and the command it invokes.
- Trace dog/deacon execution path for formulas and timeouts.
- Identify slow or blocking steps (e.g., doctor/fix, bd operations).

## Phase 1: Root-cause analysis
- Determine why session-gc exceeds 10m (CPU-bound scan, blocking CLI, missing timeouts).
- Check whether timeouts are enforced in dog runner vs. only deacon supervision.

## Phase 2: Fixes
- Implement hard timeouts for session-gc execution path.
- Optimize/skip known slow operations in session-gc or doctor.
- Add logging/metrics to surface which step exceeded time limit.

## Phase 3: Validation & docs
- Run local reproduction (dry-run) and ensure session-gc completes quickly.
- Update docs/notes about mail-session-gc behavior and timeouts.

## Acceptance criteria
- mol-session-gc no longer hangs >10m in dog runs.
- Timeouts are enforced by code (not only formula instructions).
- Deacon stability is not impacted by dog timeouts.
