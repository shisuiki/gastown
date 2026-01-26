# Roadmap

## Phase 0: Discovery
- Inspect current WebUI mail templates, handlers, and API endpoints.
- Map GT mail features to WebUI data sources (inbox, archive, queue, unread, read/unread toggle).
- Determine whether archive restore is supported in CLI/handlers.

## Phase 1: Data model + API support
- Implement/extend backend endpoints for: agents list + unread counts, queue/inbox/archive per agent.
- Add endpoints for read/unread toggle, archive, and restore if supported.
- Ensure data derived from beads/mail data directly (no CLI calls from frontend).

## Phase 2: UI refactor
- Replace mail UI layout with agent list + three-pane queue/inbox/archive.
- Implement read/unread badges + actions; archive + restore actions.
- Implement auto-refresh every 30s (dashboard-style) and remove bottom blocks.

## Phase 3: Validation + docs
- Verify mail list content matches GT mail design and states.
- Validate actions reflect in backend and UI updates.
- Update docs for WebUI mail features and behavior.

## Acceptance criteria
- New mail UI matches requested layout and functionality.
- All GT mail features are surfaced (queue/inbox/archive, read/unread, archive/restore if supported).
- Auto-refresh runs every 30s, no legacy UI sections remain.
