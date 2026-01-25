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

# Gastown Risk Checklist (Prioritized)

## P0 — Immediate Safety and Misuse Risks
- Add mandatory confirmation or `--confirm` gating for destructive gt commands (`gt down`, `gt shutdown`, `gt uninstall`, `gt rig reset`, `gt rig remove`, `gt polecat nuke`, `gt orphans kill`, `gt unsling`, `gt release`).
- Add an admin guardrail for bd destructive commands (`bd delete`, `bd admin reset`, `bd repair`, `bd migrate *`, `bd rename-prefix`, `bd resolve-conflicts`).
- Split “safe” vs “admin” modes in bd (require `BD_ADMIN=1` for admin commands).
- Add a `--dry-run` option to destructive or state-changing commands where possible.
- Provide a single `gt safe`/`gt help safe` view that lists only non-destructive commands.

## P1 — Command Taxonomy and Consistency
- Consolidate overlapping lifecycle commands into a single namespace (e.g., `gt system start/stop/status` + `gt rig start/stop/status`).
- Normalize verb usage across commands (`list` vs `show` vs `status` vs `info`).
- Rename ambiguous commands in bd (`bd q` -> `bd quick`, `bd state` -> `bd state show`).
- Explicitly document gt/bd boundary in CLI help output (“gt = agent ops; bd = data ops”).

## P1 — Prompt Safety and Clarity
- Move critical “must do” steps to the top of each role prompt.
- Extract repeated sections (Propulsion Principle, Capability Ledger) into shared includes.
- Add explicit “Do Not Do” sections per role (especially Mayor/Deacon/Witness).
- Normalize variable names in templates (avoid `{{ .Polecat }}` in crew contexts).

## P2 — Documentation and Onboarding
- Create a single “Operator Handbook” linking roles, workflows, and CLI usage.
- Provide a short “safe path” flow for new users (install -> rig add -> crew add -> mayor attach).
- Add a “dangerous commands” appendix in docs/reference.

## P2 — Tooling Clarity
- Clarify gate/park/defer semantics across gt and bd with examples.
- Add `gt` equivalents or alias guidance for the most common bd workflows.

## Scope
- Scope description pending.
