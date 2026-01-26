# Roadmap

## Phase 0: Requirements
- Inspect mail inject implementation (commit affbd27c... and current code).
- Reproduce/report conditions where tmux gets stuck in mail inject UI.
- Audit mail queue/delivery flow for dropped mail.

## Phase 1: Fix inject behavior
- Ensure inject uses prompt-style nudge, not interactive mail UI.
- Guard against running full mail TUI inside agent pane.
- Add safe fallback if inject fails or pane is busy.

## Phase 2: Fix queue/delivery reliability
- Trace mail enqueue/dequeue paths and persistence.
- Fix any missing delivery/visibility path (queue not draining, wrong dir, etc.).
- Add logs or diagnostics to surface delivery state.

## Phase 3: Validation & docs
- Verify inject delivers without blocking or leaving tmux in a TUI.
- Verify mail sent is visible and delivered to target.
- Update docs for new behavior and usage.

## Acceptance criteria
- Mail inject never spawns a blocking UI in tmux; it only nudges prompt text.
- Mail delivery is reliable and observable; no silent drop.
- Changes are documented, committed in phases, and pushed.
