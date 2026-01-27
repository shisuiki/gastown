---
type: report
status: archived
owner: "unowned"
audience: dev
applies_to:
  repo: gastown
  branch: main
last_validated: "unknown"
source_of_truth: []
---

# Gas Town WebUI Refactoring Plan (Archived)

> **Note**: This plan was largely completed. Phase 1 (shared components) was implemented with `base.html`, `gastown.css`, `gastown.js`, and `terminal.js`. Archived 2026-01-27.

This document outlines the comprehensive plan to modernize the Gas Town WebUI.

## Current State Analysis

### Code Duplication (Estimated ~500 lines duplicated)

Each of the 5 HTML templates duplicates:
- **Tailwind config** (8 lines × 5 = 40 lines)
- **Base CSS** (~45 lines × 5 = 225 lines): body, container, nav, cards, badges, scrollbars
- **Navigation HTML** (~20 lines × 5 = 100 lines)
- **Utility JS** (~10 lines × 5 = 50 lines): escapeHtml, getColorClass

### Terminology Issues

- `PolecatRow` struct used for ALL agent types (crew, polecat, refinery, patrol)
- UI text says "Polecat Terminal Viewer" but can view any agent
- Dashboard card says "Crew & Polecats" which is confusing

### Terminal Implementation Divergence

| Feature | terminals.html | mayor.html |
|---------|---------------|------------|
| Reconnect logic | Full exponential backoff | None (immediate disconnect on error) |
| Input controls | Full (text, Ctrl+C, Enter) | None |
| Session info panel | Yes | No |
| Keepalive handling | Yes (ping events) | No |

### Activity Page Gap

Current: Shows git commits (basic log output)
Needed: Beads/workflow tracking (convoys, issues, molecules, handoffs)

## Refactoring Phases

### Phase 1: Extract Shared Components

**Goal**: Eliminate 400+ lines of duplication

#### 1.1 Create Base Template (`templates/base.html`)

```html
{{define "base"}}
<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>{{.Title}} - Gas Town</title>
    <script src="https://cdn.tailwindcss.com"></script>
    <script src="https://unpkg.com/htmx.org@1.9.10"></script>
    {{template "head-extra" .}}
    <script>/* shared tailwind config */</script>
    <style>/* shared base CSS */</style>
    {{template "styles" .}}
</head>
<body>
    {{template "nav" .}}
    <div class="container">
        {{template "content" .}}
    </div>
    <script>/* shared utils: escapeHtml, getColorClass, etc. */</script>
    {{template "scripts" .}}
</body>
</html>
{{end}}
```

#### 1.2 Create Shared JS Module (`static/js/gastown.js`)

```javascript
// Core utilities
export const escapeHtml = (text) => { /* ... */ };
export const getColorClass = (color) => { /* ... */ };
export const agentTypeIcon = (type) => { /* ... */ };

// Badge/table helpers
export const priorityBadge = (p) => { /* ... */ };
export const typeIcon = (t) => { /* ... */ };

// SSE/WebSocket helpers with reconnect logic
export class ReconnectingEventSource { /* ... */ }
export class ReconnectingWebSocket { /* ... */ }
```

#### 1.3 Create Terminal Widget (`static/js/terminal-widget.js`)

Unified terminal component with:
- Connect/disconnect toggle
- SSE streaming with keepalive
- Input controls (text, special keys)
- Session info display
- Error recovery with exponential backoff

Usage:
```javascript
const terminal = new TerminalWidget('#terminal-container', {
    sessionId: 'gt-rig-agent',
    showInput: true,
    showSessionInfo: true,
});
terminal.connect();
```

### Phase 2: Rename Polecat → Agent

**Goal**: Fix misleading terminology throughout codebase

#### 2.1 Backend Changes (`internal/web/`)

| Old | New |
|-----|-----|
| `PolecatRow` | `AgentRow` |
| `polecats` (JSON field) | `agents` |
| `Polecats []PolecatRow` | `Agents []AgentRow` |

#### 2.2 API Changes

| Old Endpoint | New Endpoint |
|--------------|--------------|
| (implicit in /api/status) | `/api/agents` (list all) |
| - | `/api/agents/{id}` (single agent details) |
| - | `/api/agents/{id}/terminal` (SSE stream) |

#### 2.3 Frontend Changes

- Rename all `polecat*` IDs and variables to `agent*`
- Update labels: "Crew & Polecats" → "Active Agents"
- "Polecat Terminal Viewer" → "Agent Terminal"

### Phase 3: Replace Activity with Workflow View

**Goal**: Show beads-based workflow tracking instead of git commits

#### 3.1 New API Endpoints

```
GET /api/workflows              # List active workflows (molecules)
GET /api/workflows/{id}         # Workflow details with beads
GET /api/beads                  # List beads (filterable)
GET /api/beads/{id}             # Bead details
GET /api/convoys                # Already exists, enhance
GET /api/handoffs               # Recent handoffs
```

#### 3.2 New Activity Page Sections

1. **Active Workflows** (molecules with agents attached)
   - Molecule ID, title, assigned agent, progress
   - Click to view details (linked beads, activity log)

2. **Recent Completions** (closed beads in last 24h)
   - Bead ID, title, who closed, when

3. **Handoff Log** (recent handoffs)
   - From agent, to agent, timestamp, brief context

4. **Convoy Progress** (existing, enhanced)
   - Clickable convoy cards showing tracked issues

5. **Agent Activity Timeline**
   - Combined view of agent actions across the system

#### 3.3 Workflow Detail View (`/workflow/{id}`)

New page showing:
- Molecule metadata
- Linked beads with status
- Activity log
- Agent assignments over time

### Phase 4: Add Clickable Links & Detail Views

**Goal**: Every entity should be inspectable

#### 4.1 Entity Routes

```
/agent/{id}              # Agent detail page
/bead/{id}               # Bead detail page
/convoy/{id}             # Convoy detail page
/workflow/{id}           # Molecule/workflow detail page
/mail/{id}               # Mail message view
```

#### 4.2 Link Generation

Dashboard tables should link to detail views:
```javascript
// Current
'<td>' + escapeHtml(agent.Name) + '</td>'

// New
'<td><a href="/agent/' + agent.SessionID + '">' + escapeHtml(agent.Name) + '</a></td>'
```

#### 4.3 Breadcrumb Navigation

Detail pages include breadcrumbs:
```
Dashboard > Agents > Myrtle
Dashboard > Workflows > mol-abc123
```

### Phase 5: Expose More GT Features

**Goal**: Surface GT CLI capabilities in the WebUI

Based on `gt --help`, priority features to expose:

| Feature | Current | Planned |
|---------|---------|---------|
| `gt status` | Partial (dashboard) | Full status panel |
| `gt mail` | Partial (mail page) | Enhanced with threading |
| `gt convoy` | Partial (list only) | Create/manage convoys |
| `gt sling` | None | Dispatch work UI |
| `gt spawn` | None | Spawn new agents |
| `gt handoff` | None | Manual handoff trigger |
| `gt hook` | Partial (mayor only) | All agents |
| `gt worktree` | None | Cross-rig work UI |
| `bd` commands | Partial (issues list) | Full beads management |

#### 5.1 New Dashboard Actions

Quick action buttons:
- "Spawn Polecat" → Opens spawn dialog
- "Create Convoy" → Opens convoy wizard
- "Send Mail" → Opens mail composer
- "Manual Sync" → Triggers bd sync

#### 5.2 Agent Context Menu

Right-click or dropdown on agent cards:
- View terminal
- Send mail
- View hook status
- Trigger handoff (if crew)
- Kill session (danger)

### Phase 6: Framework Evaluation

**Current Stack**: Tailwind (CDN) + HTMX + Vanilla JS

#### Options Considered

| Option | Pros | Cons |
|--------|------|------|
| Keep HTMX | Zero build step, SSR-friendly, already in use | Limited interactivity, no components |
| Alpine.js + HTMX | Declarative, lightweight, complements HTMX | Still no component model |
| Preact/React | Rich ecosystem, reusable components | Requires build step, larger bundle |
| Svelte | Compiled, small runtime, great DX | Build step required |
| Web Components | Native, no framework lock-in | Verbose, less ecosystem |

#### Recommendation: Alpine.js + HTMX (Progressive Enhancement)

1. **Keep**: Tailwind CSS, HTMX for server interactions
2. **Add**: Alpine.js for client-side reactivity
3. **Later**: Consider Web Components for complex widgets (terminal)

This approach:
- No build step required (CDN-loadable)
- Incremental adoption (can migrate file by file)
- Server-centric architecture (matches Go backend)
- Small footprint (~15KB combined)

Example with Alpine.js:
```html
<div x-data="{ open: false }">
    <button @click="open = !open">Toggle</button>
    <div x-show="open" x-transition>Content</div>
</div>
```

## Implementation Order

1. **Phase 1** (shared components) - Reduces future work, must come first
2. **Phase 2** (rename polecat) - Quick wins, improves clarity
3. **Phase 4** (clickable links) - Low effort, high value
4. **Phase 3** (workflow view) - Core feature, needs API work
5. **Phase 5** (gt features) - Incremental, can be ongoing
6. **Phase 6** (framework) - Optional, evaluate after others complete

## File Changes Summary

### New Files

```
internal/web/
├── templates/
│   ├── base.html           # Shared layout
│   ├── partials/
│   │   ├── nav.html        # Navigation component
│   │   ├── card.html       # Card component
│   │   └── badge.html      # Badge component
│   ├── agent-detail.html   # New: agent detail page
│   ├── bead-detail.html    # New: bead detail page
│   ├── convoy-detail.html  # New: convoy detail page
│   └── workflow.html       # New: workflow/molecule page
├── static/
│   ├── js/
│   │   ├── gastown.js      # Shared utilities
│   │   └── terminal.js     # Terminal widget
│   └── css/
│       └── gastown.css     # Compiled Tailwind (optional)
├── handler_agents.go       # New: agent CRUD
├── handler_beads.go        # New: beads API
└── handler_workflows.go    # New: workflow API
```

### Modified Files

```
internal/web/
├── templates.go            # Rename PolecatRow → AgentRow
├── fetcher.go              # Update struct references
├── handler_dashboard.go    # Update to use AgentRow
├── handler_terminals.go    # Refactor to use shared terminal code
├── gui.go                  # Register new routes
└── templates/
    ├── dashboard.html      # Use base template, update terminology
    ├── terminals.html      # Use terminal widget
    ├── mayor.html          # Use terminal widget
    ├── mail.html           # Use base template
    └── activity.html       # Complete rewrite → workflow view
```

## Success Metrics

- [ ] Zero duplicate CSS/JS across templates
- [ ] All "polecat" terminology replaced with "agent"
- [ ] Terminal widget used in both terminals.html and mayor.html
- [ ] Activity page shows workflows/beads instead of commits
- [ ] Every entity (agent, bead, convoy) is clickable
- [ ] At least 5 new GT features exposed in UI
- [ ] No build step required (CDN-only dependencies)

## Open Questions

1. Should we support multiple terminal views simultaneously?
2. Do we need real-time bead updates (WebSocket) or is polling sufficient?
3. Should agent detail pages show full conversation history?
4. How to handle cross-rig work visibility in the UI?

---

*Created: 2025-01-17*
*Status: Draft - Pending Review*

## Scope
- Scope description pending.
