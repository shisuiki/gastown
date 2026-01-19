# Progress

- P0: Added same-origin checks for WebSocket status upgrades.
- P0: Added CSRF protection for state-changing requests with cookie + header enforcement.
- P0: Wrapped WebUI command execution with context timeouts.
- P1: Fixed bead creation to use bd create command.
- P1: Switched rig/mail status to structured JSON output.
- P2: Removed unused Tailwind CDN config from base template.
- P1: Terminals page now uses status WebSocket with polling fallback for agent lists.
- P1: Workflow hook status now prefers `gt hook --json` with fallback parsing.
- P1: Added mail API pagination metadata + UI load-more, and default bead list limit.
- P1: Cached agent hook API and switched hook parsing to `gt hook status --json` first.
- P2: Added mobile-friendly login link (GET token link + copy/share) on WebUI login page.
- P1: WebUI mail handlers now use mail router/mailboxes instead of `gt mail` CLI.
- P3: Added WebUI development notes doc with data layer modernization plan.
- P1: Dashboard and mayor mail counts now use mail mailbox counts instead of `gt mail` CLI.
- P1: Workflow ready list now prefers `bd ready --json` with text fallback.
