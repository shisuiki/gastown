# Roadmap

## Phase 0: UX goals + architecture
- Replace ASCII graph with visual graph rendering (canvas/SVG via OSS lib).
- Restructure Git page layout into panels (branch/commit/graph) + bottom tabs.
- Define interaction contracts: branch/commit/graph scroll sync + selection states.

## Phase 1: Graph rendering
- Integrate an open-source graph renderer (vis-network) for commit DAG.
- Adapt `/api/git/graph` output to nodes/edges for the renderer.
- Provide focus/scroll-to APIs for branch/commit selection.

## Phase 2: Layout + interactions
- Two-column layout: left top branch list, left bottom commit list, right graph.
- Bottom tabs for changes/files/compare/search with mobile stacking.
- Click interactions: branch -> graph focus, commit -> graph focus + changes, graph node -> commit scroll.

## Phase 3: UX polish + constraints
- Fix diff panel overflow and long-line constraints.
- Empty/error states, input defaults, and safe limits.
- Mobile layout verification.

## Acceptance criteria
- Graph is visual (not ASCII) and reflects branch/merge structure.
- Panel layout matches spec on desktop, stacks on mobile.
- Clicking branch/commit/graph node scrolls/focuses correctly without page navigation.
- Diff/patch views stay within bounds.
