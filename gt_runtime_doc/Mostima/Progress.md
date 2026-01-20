# Progress

## 2026-01-20
- Reworked WebUI create issue/convoy handlers to use bd with explicit BEADS_DIR and JSON parsing.
- Added shared web helpers for beads env, type normalization, and create output parsing.
- go test ./internal/web
- Reopened hq-g4w7; vendored Alpine locally and switched WebUI to /static/js/alpine.min.js for button clicks.
- Investigated unexpected mol-orphan-scan formula drift; reported to mayor with findings.

## 2026-01-21
- Investigated repeated "Work slung" tmux injections; found gt-wisp-xyz prompts in crew panes, no matching beads, and no sling timers/scripts. Logged incident report and sent findings to mayor.
- Added gt sling audit logging + allow-missing flag gate; updated sling tests and ran targeted go test.
- Disabled tmux nudges for cmd package tests via GT_TEST_NO_NUDGE in TestMain.
- Closed hq-w2c5 after confirming prefix mismatch comes from tombstones and advising skip/compact/rename workarounds to mayor.
- Switched gt sling to default skip-busy with --force-busy override and clearer busy-skip messaging.
