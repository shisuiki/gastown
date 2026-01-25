# Progress

- Pulled updated service unit using EnvironmentFile for GT_WEB_AUTH_TOKEN.
- Generated deploy token and wrote `/etc/gastown.env` with token + allow-remote.
- Copied updated unit to user systemd, reloaded, and restarted `gastown-web.service`.
