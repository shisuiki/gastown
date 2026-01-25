# Deploy Token Phase 1 Summary

## Changes
- Updated local user systemd unit for `gastown-web.service` to match new deploy file with EnvironmentFile support.
- Generated a new deploy token and stored it in `/etc/gastown.env` (with `GT_WEB_ALLOW_REMOTE=1`).
- Reloaded user systemd and restarted the web UI service.

## Rationale
- Keep the auth token out of the repo while enabling token-based access.

## Notes
- Service is running under user systemd with the updated environment file.
- Token value is delivered out-of-band to the user.
