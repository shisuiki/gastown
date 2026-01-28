# Progress Log

## 2026-01-22

### Container build - Dockerfile + docs

1. Added multi-stage `Dockerfile` that builds `gt` and runs `gt gui`.
2. Documented container run, ports/volumes, and required env vars in `docs/INSTALLING.md`.
3. Health check uses `gt version` (HTTP health check documented).
4. Local Docker validation blocked: Docker daemon socket permission denied in this environment.

### Container build - CI job

1. Added GHCR build/push job in CI for `main` and `canary` with SHA/latest/canary tags.

### Container fix - bd + ops docs

1. Added `bd` binary to container image build.
2. Added canary ops docs with container validation and beads tracking guidance.

### Prompts UI cleanup

1. Pointed CLAUDE.md editing at `mayor/CLAUDE.md` with missing indicator.
2. Restructured prompt templates by role with simplified + full prompts per role.

### WebUI git sync workflow

1. Added save-time git sync (add/commit/push) for config + prompts/CLAUDE/templates.
2. On git sync failure, auto-create a bead and sling to a polecat target.
3. Added git author env support + unsaved-change warnings for config/prompts edits.
4. Fixed config git root resolution to use `.git` instead of go.mod.

### Nightingale trigger setup

1. Added Nightingale ops doc + GitHub Actions trigger workflow and local trigger script.
2. Created `nightingale` rig + Nightingale crew workspace; agent bead creation warned about prefix mismatch.

### Canary deploy bead recording

1. Added canary deploy bead recorder script and wired `deploy/canary-deploy.sh` to log success/failure.

### Canary workflow formulas

1. Added canary deploy formula and integrated canary health checks into Deacon patrol (regenerated embedded formulas).

### Canary deploy docs

1. Documented validation, bead recording, and formula usage in canary deploy docs/checklist.

### Canary workflow summary

1. Added a summary entry for the canary workflow integration phase.

### Witness patrol wisp dedupe

1. Fixed patrol auto-spawn to detect hooked wisps and added wisp dedupe/anti-flood guardrails in witness patrol formula.

## 2026-01-21

### Docs UI tmpl support

1. Extended docs browser to include `.tmpl` files alongside markdown.

### Prompts UI template editing

1. Added CLAUDE.md editor and template file editors for system-prompts and roles on the prompts tab.

### Web UI service doc update

1. Documented current system-level `gastown-gui.service` + user-level `gastown-sync.service` split in `docs/WEBUI-DEPLOY.md`.

### Terminals auto-connect fix

1. Made `/terminals?session=...` auto-connect run only once to avoid reconnecting after manual disconnect.

### Docs env root tests

1. Added tests covering env-based docs root resolution in `internal/web/handler_docs_test.go`.

### Witness idle-but-done guard

1. Updated `mol-witness-patrol` to detect idle polecats with pushed work and nudge or submit instead of auto-nuking.

## 2026-01-20

### P2 Incident: Docs Root Regression

1. Investigated mayor commit 39f9618 vs MisumiUika fix d2633f4 and confirmed env-based docs root lookup was reverted.
2. Restored env-first docs root resolution in `internal/web/handler_docs.go`.
3. Verified `go test ./internal/web/...` after fix.

### Boot triage hook verification

1. Updated degraded Boot triage to check hook status and nudge when patrol hooks are empty.
2. Switched hook-empty detection to use `gt hook status --json`.
3. Updated Boot triage formula and Boot role template with hook status checks and idle-but-alive case.

### Handoff Summary

**WebUI Workflow Page Refactoring - Complete**

完成了workflow页面的Jira-like重构，主要工作：

1. **数据访问层重写** - 移除了所有sqlite3直接调用，改用`bd`CLI的`--json`输出
   - `beads_reader.go` - 使用`bd list/show/search --json`
   - `fetcher.go` - `getTrackedIssues()`改用`bd show --json`获取convoy dependents

2. **新API端点** (`handler_workflow_v2.go`)
   - `/api/beads` - 带过滤的beads列表
   - `/api/beads/search` - 文本搜索
   - `/api/bead/{id}` - 快速详情（直接DB访问）
   - `/api/convoy/{id}` - 快速convoy详情
   - `/api/bead/action` - 操作(sling/close/reopen/update)
   - `/api/agents/hooks` - 所有agent的hook状态

3. **前端改进** (`workflow.html`, `bead_detail.html`, `convoy_detail.html`)
   - 修复CSS变量问题（改用直接颜色值）
   - 移除UI闪烁（静默刷新）
   - 只显示有work的agent hooks
   - Bead卡片可点击跳转详情页
   - 详情页有完整操作按钮

4. **已解决的问题**
   - ✅ sqlite3命令不可用 → 改用bd CLI
   - ✅ Convoy显示0/0 → 修复getTrackedIssues使用bd show
   - ✅ Bead详情页报错 → 快速API使用bd show --json
   - ✅ UI闪烁 → 静默刷新
   - ✅ CSS样式混乱 → 移除未定义变量

### Commits
- `57418de7` feat(web): Refactor workflow page with Jira-like UI and direct beads reader
- `693115ef` fix(web): Fix workflow page CSS and eliminate UI flickering
- `705701d4` fix(web): Improve workflow UI - hide idle agents, clickable beads, fast detail pages
- `b4ffe2c7` fix(web): Replace sqlite3 with bd CLI for database access

### Known Issues / Future Work
- [ ] Convoy tracked issues可能需要进一步验证（依赖dependency_type="tracks"）
- [ ] 考虑添加更多bead操作（添加依赖、标签管理等）
- [ ] 性能优化：bd CLI调用比直接sqlite3慢，但更可靠

---

## Earlier Work (Before Handoff)

### Completed
- [x] Full codebase research completed
- [x] Understood beads data structure (9 types, SQLite + JSONL)
- [x] Understood convoy system (special bead with `tracks` dependencies)
- [x] Documentation created in gt_runtime_doc/Myrtle/

## 2026-01-28

- Reset roadmap/progress for hq-73h5f.
- Added gh proxy env overrides for WebUI GitHub API calls (CI/CD + merge queue).
- Updated WEBUI-DEPLOY docs with GT_WEB_*_PROXY env vars.
- Ran `go test ./internal/web/...` (ok).
- Committed and pushed runtime docs + gh proxy fix.
- Closed hq-73h5f and ran `bd sync`.
