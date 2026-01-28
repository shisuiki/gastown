# Summary: hq-73h5f CI/CD GitHub API Proxy Fix

## Context
WebUI CI/CD status page failed to load GitHub Actions runs because `api.github.com` resolved to a bogus benchmark IP (198.18.0.4) from the systemd service environment. Shell `gh` worked due to proxy env vars, but the service lacked them.

## Changes
- Added `ghCommand` helper to inject proxy env overrides for all `gh` invocations.
- Switched CI/CD runs (`handler_cicd.go`) and merge queue PR list (`fetcher.go`) to use `ghCommand`.
- Documented `GT_WEB_HTTP_PROXY`, `GT_WEB_HTTPS_PROXY`, `GT_WEB_ALL_PROXY`, `GT_WEB_NO_PROXY` in `docs/WEBUI-DEPLOY.md` with service examples.

## Validation
- `go test ./internal/web/...`

## Follow-ups / Risks
- If GitHub is still unreachable without a proxy, ensure `gastown-gui.service` sets the GT_WEB_*_PROXY env vars.
- Optional: add a clearer UI error when `gh` fails due to network issues.
