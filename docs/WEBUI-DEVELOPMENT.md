---
type: evergreen
status: active
owner: "unowned"
audience: dev
applies_to:
  repo: gastown
  branch: main
last_validated: "unknown"
source_of_truth: []
---

# WebUI Development Notes

This document tracks WebUI development conventions and current architecture.

## Running Locally

- Start the GUI: `gt gui`
- Start the dashboard: `gt dashboard`
- Tests: `go test ./internal/web`

## Authentication

- Web auth is enabled when `GT_WEB_AUTH_TOKEN` is set.
- Login supports:
  - Form POST with token.
  - Token login link: `/login?token=<token>` (used by the mobile share link).
- CSRF protection is enforced for state-changing requests when auth is enabled.

## Data Sources

- Status: `GET /api/status` builds a cached status snapshot.
- WebSocket: `/ws/status` pushes status updates for dashboards.
- CI/CD: `GET /api/cicd/status`, `GET /api/cicd/workflows`, `GET /api/cicd/runs/:id` (GitHub Actions via `gh`, plus canary/coldstart logs).
- Mail: WebUI uses the mail router/mailbox APIs for listing, mark read/unread, and archive.
- Beads and convoys: prefer `issues.jsonl` via `BeadsReader`/convoy fetcher with BEADS_DIR-scoped CLI fallback.
- Agent hook status: still uses `gt hook` for now (no direct hook file/state).
- Crew: `GET /api/crew/list` for rig-scoped status, `POST /api/crew/action` for lifecycle actions.

## Caching

- Stale-while-revalidate cache lives in `internal/web/cache.go`.
- Status cache TTL: 5s (see `StatusCacheTTL`).
- CI/CD status cache TTL: 5s (`CICDStatusCacheTTL`), workflow/run lists: 15s (`CICDWorkflowsCacheTTL`).
- Mail, agents, and convoys use per-endpoint caches to avoid repeated CLI calls.
- Crew cache TTL: 10s (see `CrewCacheTTL`), invalidated on crew actions.

## Frontend Patterns

- Shared styles: `internal/web/static/css/gastown.css`.
- Shared JS utilities: `internal/web/static/js/gastown.js`.
- Terminals use SSE streaming via `internal/web/static/js/terminal.js`.
- Terminal/Major history uses `ActionHistory` in `internal/web/static/js/terminal.js` with localStorage-backed state.

## Data Layer Modernization (Plan)

Current WebUI endpoints still rely on CLI-backed data reads for some lists.
Future improvements should:

1. Reduce per-request CLI calls by using cached readers.
2. Where feasible, switch to direct data access (sqlite or JSONL) to avoid CLI parsing.
3. If higher throughput is required, consider adding a Go sqlite driver and a minimal
   data access layer that reads from `.beads/beads.db` with explicit queries.

## Data Layer Modernization (Decision)

- Prefer `issues.jsonl` for read-heavy WebUI endpoints; keep BEADS_DIR-scoped CLI fallback.
- Defer Go sqlite driver adoption until JSONL access becomes a bottleneck.
- Remaining CLI dependency: agent hook status (`gt hook`) until hook state is stored on disk.

## Troubleshooting

- If WebUI appears stale, confirm the cache TTLs and status WebSocket.
- If mail actions fail, verify the town `.beads` directory and `GT_ROOT`.

## Scope
- Scope description pending.
