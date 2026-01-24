# Scratch

- Web UI probe failure note: coldstart-external-probes.sh checks :8080 inside container via netstat/ss; container may run on different port or missing tools.
- Mayor response path: canary mayor handler in /home/shisui/gt/mayor/CLAUDE.md (COLDSTART_PROBE).
- If needed, verify canary container listens with sudo docker exec gastown-canary sh -c 'netstat -tln' (netstat present; ss missing).
