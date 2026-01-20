# Progress

## 2026-01-20
- Audited systemd user units and deploy scripts; identified missing `scripts/gastown-sync.sh` and absent `gastown-gui.service`.
- Restored sync + webhook scripts, added `gastown-gui.service` alias, reloaded systemd, and restarted services.
- Logged phase summaries in `gt_runtime_doc/MisumiUika/summary/2026-01-20-service-*.md`.
- Investigated auto-redeploy failure; found system-level `gastown-gui.service` owning port 8080 and blocking user service restarts.
- Added sync-script warnings and deployment docs guidance for the system-level service conflict.
- Added a web guard script to kill stray `gt gui` processes and switched sync restarts to system-level service.
- Restarted system-level `gastown-gui.service` and reloaded sync watcher to pick up new script.
