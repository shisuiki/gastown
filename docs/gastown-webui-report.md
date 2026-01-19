# Gastown WebUI Research Report (Crew: Exusiai)

## Scope and Method
- Reviewed server-side WebUI code under `internal/web/` (handlers, templates, static assets, cache).
- Reviewed WebUI docs (`docs/WEBUI-REFACTOR-PLAN.md`, `docs/WEBUI-DEPLOY.md`).
- Did not attempt live access (auth unknown); findings are from code inspection.

## Architecture Overview
- **Server**: Go HTTP handler in `internal/web/gui.go` with routes + auth middleware.
- **Templates**: HTML in `internal/web/templates/*.html` (inline styles and inline JS).
- **Static assets**: `internal/web/static/css/gastown.css`, `internal/web/static/js/gastown.js`, `internal/web/static/js/terminal.js`.
- **Data flow**: Many endpoints call `gt`/`bd` via `exec.Command` per request.
- **Two UIs**: Convoy dashboard (`gt dashboard` and `internal/web/handler.go`) and the multi-page GUI (`gt gui`).

## Key Findings (Prioritized)

### P0 — Security and Safety
1. **WebSocket origin check is disabled**: `/ws/status` uses `CheckOrigin: true`, allowing cross-site WS connections.
   - Risk: CSRF-style attacks if cookie auth is used; accidental exposure with remote access.
2. **No CSRF protection on POST endpoints**: `/api/action`, `/api/convoy/create`, `/api/bead/create` are write operations.
   - Risk: browser-based token/cookie sessions are vulnerable to forged POSTs.
3. **Command execution in HTTP handlers without timeouts**: `exec.Command` is used with no context deadline.
   - Risk: hung commands can stall request handlers and exhaust resources.

### P1 — Reliability and Bug Risks
1. **`/api/bead/create` uses `bd new`**: The bd CLI uses `create`, not `new`.
   - Likely bug: bead creation may fail or depend on implicit aliasing.
2. **Parsing human output for status**: `getRigs()` and `getMailStatus()` parse CLI text output.
   - Risk: format drift breaks the UI silently.
3. **Mixed UI stacks with duplicated logic**: Tailwind CDN + custom CSS + inline styles, plus Alpine + HTMX + vanilla JS.
   - Result: inconsistent UI and hard-to-maintain behavior.

### P1 — Performance Risks
1. **Polling-heavy UI**: dashboard and terminals page poll `/api/status` every 5s, plus other endpoints.
   - Risk: redundant calls and excessive `gt`/`bd` execution.
2. **No pagination or virtualization**: beads, activity, and mail can grow unbounded.
   - Risk: UI slowdown and large payloads.
3. **No caching for CLI-based endpoints**: only parts of status are cached; others run commands each request.

### P2 — UX Consistency and Design Debt
1. **Inconsistent terminology**: “Polecat” vs “Agent”, “Workflow” vs “Activity”, “Dashboard” vs “GUI”.
2. **Page-level styling divergence**: many templates include custom inline styles instead of shared components.
3. **Lack of global layout patterns**: no consistent components for cards, tables, pagination, or empty states.

## Detailed Issue List (with Suggestions)

### API / Server
- **Add origin checks** for WebSockets in `internal/web/handler_dashboard.go` (match host or use allowlist).
- **Add CSRF protection** for POST endpoints (token in header or SameSite + CSRF token).
- **Wrap exec calls with context and timeouts**; return clear error states in UI.
- **Replace text parsing with structured output** (`--json` for gt/bd where possible).
- **Use centralized command runner** with strict allowlists and command-specific validation.

### UI / Frontend
- **Remove Tailwind CDN usage** in production; prebuild or replace with static CSS only.
- **Consolidate JS utilities**: keep all shared logic in `gastown.js` and avoid page-inline JS.
- **Standardize terminology** in nav labels and page titles.
- **Normalize layouts**: move inline CSS into `gastown.css` to reduce divergence.

### Performance
- **Reduce polling**: use WebSockets/SSE for status; batch endpoint calls.
- **Add pagination or limits** for large lists (`/api/beads`, mail inbox, activity).
- **Cache heavy API responses** with stale-while-revalidate patterns.

## Proposed Structural Cleanup
1. **One WebUI**: consolidate `gt dashboard` and `gt gui` into a single unified UI.
2. **Componentization**: shared page layout, shared card/table components, shared modals.
3. **API v2**: introduce explicit JSON endpoints for rigs, mail counts, agents, and convoys to avoid text parsing.

## Notes from Existing Docs
- `docs/WEBUI-REFACTOR-PLAN.md` already identifies major duplication and terminology issues; this codebase still exhibits them.
- The plan’s Phase 1 (shared base template, shared JS) appears incomplete in current templates.

## Suggested Next Steps (Actionable)
- Fix `bd new` -> `bd create` in the bead creation handler.
- Add a command execution wrapper with context timeout and structured output where available.
- Introduce CSRF middleware and WebSocket origin validation.
- Replace polling loops with SSE/WebSocket-driven updates.
- Plan a UI consistency sweep (labels, nav, and shared layout).
