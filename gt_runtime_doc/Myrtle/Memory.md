# Myrtle Memory - CI/CD Freshness Indicator (hq-m49pk)

## Current Task
Add a prominent "Last cold-start test" freshness indicator to the CI/CD page with color-coded age thresholds.

## Plan
- Find cold-start report timestamp from CI/CD status data.
- Compute relative age in the CI/CD page JS.
- Render badge with color (<6h green, <24h yellow, >24h red) and tooltip showing exact timestamp.

## Files
- `internal/web/templates/cicd.html`
- Runtime docs in `gt_runtime_doc/Myrtle/`
