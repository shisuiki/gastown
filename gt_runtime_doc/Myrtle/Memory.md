# Myrtle Memory - WebUI Workflow Refactoring

## Current Task
Refactoring WebUI workflow page to be Jira-like with full beads functionality.

## Key Discoveries

### Data Locations
- **Beads storage**: `~/gt/.beads/` (SQLite + JSONL hybrid)
- **Convoys**: Same beads system with `issue_type=convoy`
- **Routes**: `~/gt/.beads/routes.jsonl` (prefix routing)

### Bead Types (9 total)
- task (427), message (267), event (114), convoy (56), epic (29)
- bug (27), feature (14), agent (6), molecule (5)

### Current Issues
1. Hook shows 0 because `gt hook` runs without agent context
2. Convoy progress 0/0 - tracking calculation issues
3. Data fetching via CLI is slow - should read directly from beads

### File Structure
- `internal/web/handler_workflow.go` - Workflow API handlers
- `internal/web/templates/workflow.html` - Frontend template
- `internal/web/fetcher.go` - Convoy/Agent fetching logic
- `internal/web/static/js/gastown.js` - Shared JS utilities

### Key APIs Needed
- `/api/beads` - List all beads with filtering
- `/api/beads/{id}` - Single bead detail
- `/api/beads/{id}/actions` - Bead operations (sling, close, etc.)
- `/api/agents/hooks` - All agent hooks status
- `/api/convoys` - Convoy list with correct progress

## Design Decisions
1. Read directly from beads.db SQLite for performance
2. Use Alpine.js for reactive UI (already in use)
3. Jira-like board view with status columns
4. Agent dropdown for sling/assign operations
