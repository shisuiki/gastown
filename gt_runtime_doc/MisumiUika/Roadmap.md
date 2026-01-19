# Roadmap

## Phase 0: Load context and triage
- Locate/read `docs/gastown-webui-report.md` and extract issues.
- Map current WebUI architecture (auth flow, data sources, CLI usage).
- Decide target data sources (existing sqlite vs new db) and constraints.

## Phase 1: Fix report issues (urgent to non-urgent)
- Address each issue in order of urgency.
- One issue per commit with focused tests/notes.

## Phase 2: Mobile-friendly authentication
- Research options that fit the repo (passkeys, magic links, QR-based login).
- Implement the chosen flow and update WebUI templates/handlers.

## Phase 3: Replace CLI-driven data population
- Identify handlers calling `gt`/`bd` CLI or shelling out.
- Replace with direct data access (state/sqlite/files) where feasible.
- Keep behavior parity and update tests.

## Phase 4: Data layer modernization
- Evaluate existing sqlite usage (gt/bd) for reuse.
- If insufficient, propose and implement a new db + minimal backend.

## Phase 5: Documentation + review
- Maintain a Web development manual during refactor.
- Perform a post-refactor code review and capture findings.
