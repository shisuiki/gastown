# Doc Head Skill

Every docs change (new file or significant update to `docs/**/*.md`, especially runbooks/operations notes) runs through this skill so the YAML frontmatter stays uniform and self-describing.

## When to apply
- Adding or fixing any `docs/**/*.md` file (concepts, operations, templates, etc.).
- Updating a runbook or procedure (deploy, validation, failure handling, CI/CD workflow).
- Touching reports or research notes: switch them to the `report` archetype and archive when completed.

## Standard head template
```yaml
---
type: <evergreen|runbook|report|example>
status: <active|deprecated|archived>
own­er: "unowned"
audience: <dev|ops|oncall>
applies_to:
  repo: gastown
  branch: <main|canary|unknown>
last_validated: "unknown"
source_of_truth:
  - "<repo path for script/config>"
  - "..."
---
```
- `type`+`status` are mandatory.
- `owner` and `audience` default to `unowned`/`dev` when unsure; update later when a human owner exists.
- `applies_to.branch` reflects the branch this doc supports (`canary` if tied to the canary workflow, `main` otherwise, `unknown` when both or future uncertain).
- `last_validated` = `unknown` until someone signs off.
- `source_of_truth` lists the repo paths (scripts, configs, workflows) that prove the doc; leave an empty array (`[]`) if unknown.

## Document type defaults
| Type | Description | status default | audience | extra fields |
| --- | --- | --- | --- | --- |
| `evergreen` | Stable references (architecture, glossary, tutorials). | `active` | `dev` | include `## Scope` section if missing. | 
| `runbook` | Operational procedures (deploy, validation, failure, CI/CD). | `active` | `ops` (or `oncall`/`dev` if clearly targeted). | add `ttl_days`, `next_review`, `## Preconditions`, `## Steps`, `## Verification`. | 
| `report` | Historical research/incident notes. | `archived` | `ops` | move file under `docs/archive/YYYY-MM/` and keep stub in original path pointing to the archive (see templates). Do **not** add `ttl_days`. |
| `example` | Demos or walkthroughs. | `active` | `dev` | keep short, no TTL. |

When in doubt, default to `evergreen` for ongoing reference or `runbook` for actionable procedures. Use `report` only for one-off investigations, and place it under `docs/archive/YYYY-MM/` with the stub referencing the archived copy.

## Runbook checklist
1. `ttl_days` ≥ `30`, `next_review` present, `Verification` section describing how to confirm the steps without external dependencies.
2. `source_of_truth` points to scripts/workflows (e.g., `deploy/canary-deploy.sh`, `.github/workflows/canary-deploy.yml`).
3. Section skeleton: `## Preconditions`, `## Steps`, `## Verification`.
4. Prefer citing scripts/configs, don’t hardcode ports/paths unless documented elsewhere.
5. Use `unknown`/`unowned` when metadata is missing; document how to fill it later.

## Anti-rot writing rules
- Never hardcode changing facts (ports, hosts, command arguments). Always cite the `source_of_truth` script or config.
- If you mention an environment variable, verify it exists via `rg` in `deploy/`, `scripts/`, or `.github/workflows` and reference it in `source_of_truth`.
- For commands, point to the actual script that runs them; do not copy-paste entire output.
- Keep the head minimal; extra prose belongs in the body or templates.
- For archived reports, keep a stub at the old path that points readers to `docs/archive/YYYY-MM/...`.

## Writing checklist
1. Frontmatter present with `type`, `status`, `owner`, `audience`, `applies_to`, `last_validated`, and `source_of_truth` (array).
2. Type matches the doc intent (evergreen/runbook/report/example).
3. Runbooks include `ttl_days` + `next_review` + the three required sections.
4. If referencing scripts/configs, list them in `source_of_truth` (no external URLs).
5. Confirm body has a `## Scope` (evergreen) or `## Verification` (runbook).
6. If doc is report-style, remember to archive it in `docs/archive/YYYY-MM/`.
7. Avoid embedding volatile facts; point readers to the source script/list instead.
8. Leave `unknown`/`unowned` when unsure and note who should fill it later.
9. Gently point auther to `docs/_templates/*` or `skills/doc_head.md` for instant copies.
10. Run `scripts/docs_lint.sh` if available before finishing.

## Examples
### Runbook head
```yaml
---
type: runbook
status: active
owner: "unowned"
audience: ops
applies_to:
  repo: gastown
  branch: canary
last_validated: "unknown"
ttl_days: 30
next_review: "unknown"
source_of_truth:
  - "deploy/canary-deploy.sh"
  - ".github/workflows/canary-deploy.yml"
---
```
### Report head
```yaml
---
type: report
status: archived
owner: "unowned"
audience: ops
applies_to:
  repo: gastown
  branch: unknown
last_validated: "unknown"
source_of_truth:
  - "docs/archive/2026-01/DOG-TIMEOUT-FIX-REPORT.md"
---
```

Keep this skill nearby `skills/doc_head.md` so every agent and teammate can copy it before touching docs. If metadata changes later (owner, branch, TTL), update the head and rerun `scripts/docs_lint.sh`.
