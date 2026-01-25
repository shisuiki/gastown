# Deploy Token Final Summary

## What changed
- Deployment now uses `/etc/gastown.env` for `GT_WEB_AUTH_TOKEN` and allows remote access.
- `gastown-web.service` was reloaded and restarted using the updated unit file.

## Why
- Enables secure token-gated access without committing secrets.

## Risks / Follow-ups
- If you want to lock back to localhost-only access, remove `GT_WEB_ALLOW_REMOTE=1` and restart the service.
- Keep `/etc/gastown.env` permissions restrictive (600) to avoid token exposure.
