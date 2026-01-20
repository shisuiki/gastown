# Progress

## 2026-01-20
- Audited systemd user units and deploy scripts; identified missing `scripts/gastown-sync.sh` and absent `gastown-gui.service`.
- Restored sync + webhook scripts, added `gastown-gui.service` alias, reloaded systemd, and restarted services.
