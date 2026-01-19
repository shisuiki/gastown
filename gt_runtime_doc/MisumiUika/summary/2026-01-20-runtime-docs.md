# Summary: runtime docs protocol in role templates (2026-01-20)

## Phase 0
- Created `gt_runtime_doc/MisumiUika/` with Memory, Roadmap, Progress scaffolding.

## Phase 1
- Reviewed role templates in `internal/templates/roles/` to find insertion points.

## Phase 2
- Added a runtime docs protocol section to all role templates (crew, mayor, polecat, witness, refinery, deacon, boot).

## Phase 3
- Ran `go test ./internal/templates` to validate template rendering.

## Final summary
- All role prompts now include a consistent runtime docs workflow (Memory/Roadmap/Progress/summary, update + commit rules, and escalation guidance).
