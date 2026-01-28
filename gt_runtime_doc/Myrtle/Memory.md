# Myrtle Memory - WebUI CI/CD GitHub API Connectivity (hq-73h5f)

## Current Task
Fix WebUI CI/CD GitHub API connectivity where `gh` calls time out due to DNS/proxy mismatch in systemd service.

## Key Findings
- `gh run list` in WebUI resolves `api.github.com` to `198.18.0.4` and times out.
- Shell `gh` works because proxy env vars are set; systemd service lacks them.
- Root cause likely DNS interception from `Meta` interface + no proxy in `gastown-gui.service`.

## Implementation Plan
- Add `ghCommand` helper that injects proxy env overrides for `gh` calls only.
- Env vars: `GT_WEB_HTTP_PROXY`, `GT_WEB_HTTPS_PROXY`, `GT_WEB_ALL_PROXY`, `GT_WEB_NO_PROXY`.
- Update WebUI docs with proxy env config for `gastown-gui.service`.

## Files Touched
- `internal/web/command.go`: new `ghCommand` + env override helpers
- `internal/web/handler_cicd.go`: use `ghCommand`
- `internal/web/fetcher.go`: use `ghCommand` for PR list
- `docs/WEBUI-DEPLOY.md`: document proxy env vars
