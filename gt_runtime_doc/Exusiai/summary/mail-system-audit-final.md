# Mail System Audit Final Summary

## What changed
- Mail injection now supports a nudge path for background retries and uses per-session markers with explicit identities.
- Busy-session notifications schedule deferred injects rather than silently waiting.
- Mail delivery/inject behavior is documented in the mail protocol design doc.

## Why
- Fixes cases where deferred injects ran in the wrong context (overseer inbox) and never notified recipients.
- Eliminates blocked tmux UI by nudging instead of running interactive commands in the pane.

## Follow-ups
- If you want hook-only behavior, keep using `gt mail check --inject` in Claude hooks.
- If notifications still feel delayed, adjust `mailInjectIdleThreshold` or `mailNotifyIdleThreshold`.
