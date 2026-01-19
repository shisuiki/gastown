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
