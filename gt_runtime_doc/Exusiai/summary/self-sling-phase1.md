# Self-sling Phase 1 Summary

## Changes
- Added a `--self` flag to `gt sling` and a self-sling guard that blocks self-target slings unless explicitly confirmed.
- Added a warning output that explains the impact and how to delegate instead.
- Updated sling tests that sling formulas to self so they set `slingSelf = true` and restore it afterward.

## Rationale
- Prevents accidental self-slinging while preserving an explicit override for intentional self-assignments.

## Notes
- Self-sling guard allows `--dry-run` to continue after warning, preserving non-destructive inspection flows.
- No external docs updated beyond CLI flag help.
