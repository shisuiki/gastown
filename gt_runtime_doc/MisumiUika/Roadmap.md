# Roadmap

## Phase 0: Service inventory
- [x] Inspect user systemd units and deployment scripts.
- [x] Identify missing or renamed services and broken paths.

## Phase 1: Restore sync script
- [x] Ensure `gastown-sync.sh` exists at the path used by systemd.
- [x] Align webhook script path with docs.

## Phase 2: Service compatibility
- [x] Restore or alias `gastown-gui.service` to the current web service.
- [x] Reload and restart affected units.

## Phase 3: Verification
- [x] Confirm sync service can run on restart.
- [x] Validate web service restart on sync.

## Phase 4: Conflict guardrails
- [x] Add warnings when a system-level service blocks user-level auto-redeploy.
- [x] Document port ownership conflicts in deployment docs.
