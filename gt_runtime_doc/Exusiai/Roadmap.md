# Roadmap

## Phase 0: Git Web UI requirements and gaps
- Inventory current Git Web UI functionality, missing interactions, and data gaps.
- Define the minimum complete Git feature set (graph, commit detail, diff, file tree, branch view, compare, search).
- Confirm data sources, API shapes, and UX flow for Git operations.

## Phase 1: Backend Git APIs (data completeness)
- Commit detail API (metadata, parents, refs, stats).
- Commit diff API (file list + unified diff).
- File tree API (tree listing at ref/path) and file content API (blob view).
- Branch metadata (ahead/behind, last commit, tracking info).
- Graph API returns real DAG / graph text, not placeholder.

## Phase 2: Core Git UI (interactive navigation)
- Commit list with selection state and details panel.
- Commit diff view with file list + inline patch.
- File browser (tree + blob viewer) with path breadcrumbs.
- Branch list with filtering, current branch, tracking status.

## Phase 3: Graph + compare + search
- Real graph visualization (ASCII graph or DAG columns) tied to commits.
- Compare view (branch/ref/commit range) with diff and stats.
- Search (commit message/author + path-limited grep) and results list.

## Phase 4: UX polish + performance
- Pagination/virtualization where needed.
- Caching and safe limits for git commands.
- Error states and empty states for every panel.

## Acceptance criteria
- Users can browse branches, graph, commit details, diffs, and file tree.
- Users can click commits and inspect changes.
- Graph reflects actual branch/merge history.
- Compare and search are usable and stable.
