# Roadmap: WebUI Workflow Refactoring

## Phase 1: Data Layer (Current)
- [x] Research current implementation
- [x] Research beads data structure
- [x] Research bd/gt CLI tools
- [ ] Create direct beads.db reader
- [ ] Fix convoy progress calculation
- [ ] Implement per-agent hook fetching

## Phase 2: Core UI Components
- [ ] Beads list view with filtering (by type, status, priority)
- [ ] Bead detail page with full info
- [ ] Agent hooks overview (per-agent)
- [ ] Convoy detail page with clickable beads

## Phase 3: Bead Operations
- [ ] Create new bead form
- [ ] Update bead (status, priority, assignee)
- [ ] Sling bead to agent
- [ ] Close/reopen bead
- [ ] Add dependencies

## Phase 4: Board View (Jira-like)
- [ ] Kanban board by status (open/in_progress/closed)
- [ ] Drag-and-drop status change
- [ ] Quick filters (my work, ready, blocked)
- [ ] Search functionality

## Phase 5: Polish
- [ ] Keyboard shortcuts
- [ ] Bulk operations
- [ ] Real-time updates via SSE
- [ ] Mobile responsive improvements

## Success Criteria
- All bd CLI operations available in WebUI
- Faster than CLI (direct DB access)
- Intuitive Jira-like interface
- Per-agent hook visibility
