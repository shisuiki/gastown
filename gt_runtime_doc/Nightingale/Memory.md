# Memory

- hq-8ynf4 DEPLOY task (proxy fix) completed (TEST_ID: coldstart-20260124-175829).
- Container rebuilt from canary branch (7dda3265 - proxy fix).
- Proxy fix verified: container can reach api.anthropic.com through host.docker.internal:7890.
- Results: external 6/7 PASSING (gt_status timeout), internal AUTH_EXPIRED.
- Report: /home/shisui/gt/logs/coldstart-tests/coldstart-20260124-175829.json
- Mayor session (hq-mayor) now starts and persists.
- Blocking issue: Claude OAuth token expired (401 authentication_error).
- Needs manual re-authentication via /login in container.
- Fixed: .claude directory permissions (chown to uid 10001).
