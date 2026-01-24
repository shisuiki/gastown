# Memory

- hq-rxziu DEPLOY task completed (TEST_ID: coldstart-20260124-155857).
- Container rebuilt from canary branch (gastown:canary-full-5cb8314d1640).
- Results: external PASSING (7/7), internal NO_RESPONSE from canary mayor.
- Report: /home/shisui/gt/logs/coldstart-tests/coldstart-20260124-155857.json
- web_ui now passing (was failing in previous test coldstart-20260124-132053).
- Issues discovered:
  - gt mayor start uses invalid --no-news flag (Claude Code 2.1.19 doesn't have it)
  - Claude credentials permissions were wrong (fixed: chown to uid 10001)
  - Beads directory permissions were restrictive (fixed: chmod o+rw)
- Previous follow-ups still open:
  - hq-t00sx: web_ui probe failure (now resolved by container rebuild)
  - hq-ifo2u: mayor NO_RESPONSE (root cause identified: --no-news flag bug)
