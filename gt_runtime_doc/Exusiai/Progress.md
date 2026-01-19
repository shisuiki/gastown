# Progress

- Added prompt partials and reordered role templates with critical rules and Do-Not-Do sections.
- Added AgentName to role data and updated crew/polecat templates for naming clarity.
- Aligned system prompts with role boundaries and commit-granularity guidance.
- Added --confirm guardrails to high-risk gt commands and documented new requirement.
- Closed prompt optimization issues: hq-zqyl, hq-5abf, hq-gpxx, hq-dy59, hq-u9lw.
- Ran `go test ./internal/templates`.
- Rebuilt `gt` from repo to include quoting fix; `gt mayor start` now succeeds.
- Fixed auto-sync chain: updated post-merge hook to restart user-level web service, added PATH/HOME to sync service, enabled linger, added user logrotate timer for sync log, restarted web service under systemd.
