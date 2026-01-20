# Roadmap

## Phase 0: Service inventory
- Inspect user systemd units and deployment scripts.
- Identify missing or renamed services and broken paths.

## Phase 1: Restore sync script
- Ensure `gastown-sync.sh` exists at the path used by systemd.
- Align webhook script path with docs.

## Phase 2: Service compatibility
- Restore or alias `gastown-gui.service` to the current web service.
- Reload and restart affected units.

## Phase 3: Verification
- Confirm sync service can run on restart.
- Validate web service restart on sync.
