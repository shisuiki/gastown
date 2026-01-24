# Roadmap

## Phase 0: Requirements
- Confirm desired behavior: delay reminders without dropping them.
- Identify safe way to reschedule inject checks inside tmux without spamming.
- Define a marker mechanism to avoid multiple pending timers.

## Phase 1: Delayed injection scheduling
- When inject is skipped due to activity, schedule a delayed retry (sleep 120s).
- Store a runtime marker with next scheduled time to avoid duplicate timers.
- Clear marker once a reminder is successfully injected.

## Phase 2: Validation
- Ensure inject mode remains silent when unread=0.
- Ensure delayed retry re-checks idle state and re-schedules if still active.
- Ensure non-inject modes are unchanged.

## Acceptance criteria
- Reminders are delayed (not dropped) until tmux idle for 2 minutes.
- Only one pending retry is scheduled per session.
