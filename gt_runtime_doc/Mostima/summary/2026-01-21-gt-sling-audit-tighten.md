# Summary: gt sling audit logging and allow-missing gate

Date: 2026-01-21

Context
- User reported repeated "Work slung: gt-wisp-xyz" prompts with empty hooks.
- Action required: add audit logging for gt sling calls and tighten slinging of unverified bead IDs.

Changes
- Added audit logging for gt sling calls (bead/target/pane/session/cwd/pid/BEADS_DIR/flags) via .events.jsonl audit entries.
- Introduced `--allow-missing` to gate fallback slinging of bead-like IDs that fail verification.
- If `--allow-missing` is used and bead info lookup fails, sling continues without pinned/convoy checks.
- Added tests for the new behavior and updated sling tests/comments.

Tests
- go test ./internal/cmd -run 'TestSlingRejectsMissingBeadWithoutAllowMissing|TestSlingAllowMissingSkipsBeadInfo|TestLooksLikeBeadID'
