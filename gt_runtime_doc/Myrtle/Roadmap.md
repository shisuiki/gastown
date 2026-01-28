# Roadmap: WebUI CI/CD GitHub API Connectivity (hq-73h5f)

## Phase 1: Triage + Root Cause
- [x] Reproduce WebUI CI/CD failure (gh run list timeouts)
- [x] Identify network/DNS differences between service and shell
- [x] Confirm proxy/env gaps in service runtime

## Phase 2: Fix
- [x] Implement proxy/env propagation for `gh` calls in WebUI
- [ ] Add safe fallback or helpful error when GH API unreachable

## Phase 3: Docs + Validation
- [x] Update WebUI deploy docs with required proxy/DNS env
- [ ] Add/adjust tests if needed
- [x] Verify behavior locally (targeted go test)

## Phase 4: Land + Sync
- [x] Commit + push
- [x] Update Progress + Memory
- [ ] `bd sync` and close bead if done
