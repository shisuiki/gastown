# Roadmap

## Phase 0: CI/CD UX + data plan
- [ ] Review dashboard layout + dev menu placement for CI/CD surfaces.
- [ ] Define data sources (GitHub Actions, coldstart logs, canary validation).
- [ ] Draft API payloads for compact card + detailed page.

## Phase 1: Backend CI/CD endpoints
- [ ] Add handler for /api/cicd/status, /api/cicd/workflows, /api/cicd/runs/:id.
- [ ] Implement StatusCache-backed aggregation with 5s TTL.
- [ ] Wire logs ingestion (coldstart + canary validation) into summaries.

## Phase 2: Dashboard compact card
- [ ] Add CI/CD card to dashboard grid with health rollup + recent runs.
- [ ] Update WS update flow to refresh CI/CD snapshot.
- [ ] Link to detailed /cicd page.

## Phase 3: CI/CD detail page
- [ ] Create /cicd template with workflow list + run history.
- [ ] Add filter controls (workflow, branch, status).
- [ ] Add report text blocks for internal/external assessments.

## Phase 4: Docs + validation
- [ ] Update WEBUI-DEVELOPMENT.md with new endpoints and data sources.
- [ ] Run /internal/web tests or targeted checks.
