# Self-sling Final Summary

## What changed
- `gt sling` now warns on self-target slings and requires `--self` to proceed.
- Self-sling attempts print a short guidance block with delegation examples.
- Sling tests for formula-on-bead flows now opt into self-sling confirmation.

## Why
- Avoids accidental self-sling interruptions while preserving an explicit override for intentional self-assignments.

## Risks / Follow-ups
- If operators relied on `gt sling <bead>` without specifying `--self`, this is now a hard-stop. If this proves too strict, consider adding a softer warn-only mode behind a config toggle.
