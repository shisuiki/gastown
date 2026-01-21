# Progress

## 2026-01-21
- Added shared history logger in terminal JS with pagination controls.
- Wired Terminal and Mayor pages to record send/key actions into separate histories.
- Ensured Send defaults to include Enter and documented behavior as needed.
- Adjusted terminal send to append carriage return for reliable Enter delivery.
- Switched terminal send to debounced tmux Enter to trigger prompt submission.
- Added length-aware debounce for tmux nudges to reduce missed Enter on sling/mail.
