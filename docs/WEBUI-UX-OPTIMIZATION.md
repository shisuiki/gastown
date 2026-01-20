# WebUI UX Optimization Recommendations

## Goals
- Reduce cognitive load when rigs, agents, and beads scale.
- Normalize interaction patterns across pages.
- Make primary work flows (beads, convoys, hooks, mail) discoverable with less noise.
- Create progressive disclosure: summary first, detail on demand.

## Global Information Architecture

### Proposed Top-Level Navigation
Current nav: Dashboard, Mayor, Mail, Terminals, Workflow, Git, Config, Prompts.

Recommended nav:
1) Overview
2) Work
3) Agents
4) Comms
5) Repo
6) Settings

Mapping:
- Overview = current Dashboard (trimmed to signals + actions).
- Work = current Workflow (board/list/ready/convoys) + convoy detail entry point.
- Agents = current Terminals + agent hooks + role control (replaces Mayor page).
- Comms = current Mail (with filters and bulk actions).
- Repo = current Git.
- Settings = Config + Prompts (sub-tabs).

Rationale:
- Removes Mayor as a special-case page; roles should be first-class across agents.
- Groups settings under one place to reduce nav clutter.
- Creates a clear “Work vs Operations vs Settings” split.

### Global Scoping and Search
- Add a global scope selector (Town / Rig / Crew) in the nav. Default to Town.
- Add a global search entry (beads, convoys, agents, mail) with keyboard shortcut.
- Persist filters per page so returning users do not reconfigure each view.

### Cross-Page Interaction Patterns
- Prefer list summaries with counts, then expand to details.
- For long lists, default to top 20 with “Show more” and server-side pagination.
- Use consistent empty states: “Why empty?” + “What can I do now?”
- Unify badges (priority, status, agent type) with a shared legend.
- Convert inline actions into compact action bars (primary/secondary).
- Add bulk actions where repeated operations occur (sling, assign, close).

## Page-by-Page Recommendations

### Overview (Dashboard)
Keep:
- Rigs, Convoys, Active Agents as compact summaries with counts.
- System + Claude usage in an “Ops Metrics” section.

Adjust:
- Replace the large grid with two columns: “Operations Summary” and “Work Summary”.
- Convert “Active Beads” into a 3-part summary: P1/P2 count, Ready count, Blocked count.
- Convert “Recent Activity” to a compact activity feed with filters (beads, git, mail).
- Move “Daemon Status / BD Sync / Ready” actions into a command palette.

Remove or merge:
- Role Beads card (fold into Agents or Work summary).
- Mail card content; replace with count + quick action “Open Comms”.

Add:
- Alerts panel: “Blocked convoy”, “No hooks”, “Mail backlog”, “Refinery queue”.
- “Next Actions” list: top 3 ready beads with recommended agent type.

Scalability:
- All cards show only top N entries and link to detail pages.

### Work (Workflow)
Keep:
- Board, List, Ready, Convoys tabs.

Adjust:
- Add filters for Rig, Assignee, Label, and Agent Type.
- Add a “Group by” selector for List view (status, assignee, type).
- Default Board to Open + In Progress; hide Closed behind a toggle.
- Add “Blocked” and “Ready” columns as optional board variants.
- Show dependency count and blocker reason on cards.

Remove or merge:
- Remove duplicate “Active Convoys” from sidebar when Convoys tab is active.

Add:
- Bulk actions in List and Ready (sling, assign, close).
- “Quick create” inline form (title + type) instead of a modal for power users.
- “Suggested assignee” based on agent type or last owner.

Scalability:
- Virtualize long lists, paginate closed items, and keep search scoped.

### Bead Detail
Keep:
- Primary action bar (sling, assign, close).
- Dependencies section.

Adjust:
- Move “Change Priority” and “Assign” into a side panel to reduce modal churn.
- Promote status and assignee to top meta block.
- Add “Blocked by” reasons and dependency resolution hints.

Add:
- Activity timeline (status changes, assignee changes, sling events).
- Related convoys and linked beads.

### Convoy Detail
Keep:
- Progress bar and tracked issues.

Adjust:
- Add “Owner / Lead agent” and “Last activity” metadata.
- Collapse tracked issues by default; show counts per status.

Add:
- “Auto-sling ready issues” button.
- “Blocked issues” subsection with reasons.

### Agents (Terminals + Hooks + Roles)
Keep:
- Terminal streaming and session input.

Adjust:
- Add a left rail list of agents with search + filters (rig, type, role).
- Add quick actions per agent row: open terminal, send mail, sling bead.
- Show hooks in the agent list, not a separate card.

Replace Mayor page with Role Control:
- A grid of role cards (Mayor, Deacon, Witness, Refinery, etc.).
- Each card shows: status, hook state, mail count, terminal link, last activity.
- Allows sending messages to any role, not just Mayor.

Scalability:
- Group agents by rig and collapse sections.

### Comms (Mail)
Keep:
- Agent-based inbox selection and compose.

Adjust:
- Split into two tabs: Inbox and Compose.
- Add filters: Unread, Agent Type, Rig, Time range.
- Replace full message bodies with preview + expand-on-click.
- Move Mark Read/Unread/Archive into row actions.

Add:
- Bulk selection and actions.
- Saved templates for common messages.

Scalability:
- Pagination and search by sender/subject.

### Repo (Git)
Keep:
- Commits and Branches tabs.

Adjust:
- Default to Commits; hide Graph behind “Advanced”.
- Add search (by author, message).
- Show working tree status (dirty, ahead/behind) in header.

Add:
- Per-rig summary tiles (commits today, open PRs, failed CI).

### Settings (Config + Prompts)
Keep:
- Role agent assignment, custom agents, prompt editing.

Adjust:
- Split into tabs: Agents, Models, Prompts, System.
- Replace free-text agent selection with dropdowns using available agents.
- Add inline validation for model/endpoint/auth profile.

Add:
- Effective prompt preview (resolved content + source chain).
- Diff view between Town-level and Rig-level overrides.

### Legacy Convoy Dashboard (/convoy)
- Either remove this page or wrap it with the main nav and shared styles.
- Redirect to Work > Convoys to avoid duplication.

## Roadmap (Implementation Order)

Phase 1: IA and Navigation
- Add global scope selector and unify nav.
- Move Mayor into Agents (Role Control).
- Merge Config + Prompts under Settings.

Phase 2: Work and Agent Scaling
- Add filters, grouping, and bulk actions to Work.
- Add agent list rail + filters to Agents.
- Introduce pagination and limits for large lists.

Phase 3: Comms and Detail Pages
- Improve Mail with inbox/compose split, previews, bulk actions.
- Add timeline and related items to bead/convoy detail pages.

Phase 4: Overview and Repo polish
- Rebuild Dashboard with alerts + next actions.
- Add Repo summary tiles and search.

## Success Metrics
- Time to locate a specific bead/agent under 10 seconds.
- Work page remains responsive with 500+ beads.
- Mail and agent lists remain usable with 200+ agents.
- Primary actions accessible within 2 clicks from any page.
