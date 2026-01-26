# Phase 1 Summary - WebUI Mail Refactor

## Scope
- Implemented WebUI mail layout refactor with agent list + queue/inbox/archive panes.
- Added backend aggregation to support queue visibility and unread counts without CLI calls from frontend.
- Updated WebUI docs to describe the new mail data endpoint.

## Changes
- Added `GET /api/mail/view` endpoint that returns queue, inbox, and archive data per agent, including totals and limited flags.
- Extended `/api/agents/list` to return unread counts and queue indicators derived from beads JSONL and queue beads.
- Built mail index from `issues.jsonl` via `BeadsReader` to avoid per-request CLI calls and to compute queue/unread data.
- Replaced `mail.html` UI with a new layout: left agent list, right three columns, auto-refresh every 30s, explicit read state labels, and archive action.
- Added mail-specific CSS for the new layout and responsive single-column mode on mobile.
- Documented the new mail data source in `docs/WEBUI-DEVELOPMENT.md`.

## Notes / Risks
- Archive restore is not implemented because GT mail does not expose restore; archive view is read-only.
- Queue list shows unclaimed queue messages matched via claim patterns; claimed items are excluded.
- Auto-refresh uses existing cache TTLs; unread counts may be stale for up to the cache TTL.
