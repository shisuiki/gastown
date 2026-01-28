# Summary: hq-m49pk CI/CD Freshness Indicator

## Context
The CI/CD page lacked any indication of how old the cold-start results were, making it unclear whether data was recent.

## Changes
- Added a cold-start freshness badge on the CI/CD page header.
- Badge reads "Last cold-start test: …" and color-codes by age (<6h green, <24h yellow, >24h red).
- Tooltip shows the exact timestamp (from cold-start report `updated_at`), with a fallback "unknown" when missing.

## Validation
- Logic verified in template (no browser run).

## Follow-ups
- Optionally verify in the live UI after new cold-start results are generated.
