# Roadmap

## Phase 0: Crew WebUI UX + data plan
- [ ] Map `gt crew` CLI commands to WebUI actions and API contracts.
- [ ] Define crew list/status payloads and cache strategy.
- [ ] Draft Crew page UX (filters, actions, modals, terminal link).

## Phase 1: Crew data endpoints
- [ ] Add cached crew list/status APIs with rig-aware aggregation.
- [ ] Include session status, git clean/dirty, mail counts, and terminal session IDs.

## Phase 2: Crew action endpoints
- [ ] Implement add/remove/rename/start/stop/restart/refresh/pristine actions.
- [ ] Validate inputs and clear caches after state changes.

## Phase 3: Crew WebUI page
- [ ] Add new Crew tab and template with rig filters and action controls.
- [ ] Wire JS to fetch list/status and trigger actions with output logging.
- [ ] Provide terminal jump links for running sessions.

## Phase 4: Docs + tests
- [ ] Update `docs/WEBUI-DEVELOPMENT.md` with Crew page and API notes.
- [ ] Add handler tests for crew list/actions where feasible.
