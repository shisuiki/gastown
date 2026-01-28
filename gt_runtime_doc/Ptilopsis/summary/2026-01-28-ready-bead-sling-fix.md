# Summary: Ready Bead Sling Fix (hq-kch75)

## What changed
- Updated `/api/bead/action` to run `gt` commands from the town root and `bd` commands with the web beads environment.
- This ensures sling actions launched from the Ready beads UI resolve the correct rig and no longer fail with a bad working directory.

## Why
- The WebUI was invoking `gt sling` from the web server's current directory, which could be a non-rig path (e.g., `.../rigs/...`).
- `gt` then mis-identified the rig ("rigs"), so the Sling button appeared to do nothing.

## Tests
- `go test ./internal/web/...`
