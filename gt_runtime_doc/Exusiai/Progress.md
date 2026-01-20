# Progress

- Cleared for Git UI graph/layout/interaction overhaul.
- Added vis-network assets and branch commit hash metadata to support visual graph focus.
- Rebuilt Git UI layout with panels and tabs, added visual graph interactions and scroll syncing, and constrained diff overflow.
- Added prompt partials and reordered role templates with critical rules and Do-Not-Do sections.
- Added AgentName to role data and updated crew/polecat templates for naming clarity.
- Aligned system prompts with role boundaries and commit-granularity guidance.
- Added --confirm guardrails to high-risk gt commands and documented new requirement.
- Closed prompt optimization issues: hq-zqyl, hq-5abf, hq-gpxx, hq-dy59, hq-u9lw.
- Ran `go test ./internal/templates`.
- Rebuilt `gt` from repo to include quoting fix; `gt mayor start` now succeeds.
- Fixed auto-sync chain: updated post-merge hook to restart user-level web service, added PATH/HOME to sync service, enabled linger, added user logrotate timer for sync log, restarted web service under systemd.
- Adjusted graph rendering to fit within fixed height, auto-fit view, and improve layout spacing.
- Reworked git graph API to return node list (no ASCII parsing) and updated graph panel rendering with fixed sizing.
- Replaced vis-network graph with custom SVG lane renderer, fixed scroll sync and styling for readable graph rows.
