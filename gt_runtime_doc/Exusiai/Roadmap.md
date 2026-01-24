# Roadmap

## Phase 0: Requirements
- Inspect current mail injection path (`gt mail check --inject`) and hook usage.
- Identify reliable tmux idle signal for 2-minute inactivity.
- Decide on fallback behavior when idle cannot be determined.

## Phase 1: Mail injection gating
- Add idle-time check before emitting injected mail reminders.
- Use tmux session activity or last-attached timestamps to compute idle time.
- Make inject mode silent if idle threshold not met.

## Phase 2: Validation
- Ensure non-inject modes are unchanged.
- Ensure inject mode remains non-fatal and silent on errors.
- Confirm idle threshold is 2 minutes.

## Acceptance criteria
- `gt mail check --inject` emits reminders only after 2 minutes of tmux inactivity.
- Active sessions are not interrupted by mail injection.
