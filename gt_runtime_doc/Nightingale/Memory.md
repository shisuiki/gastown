# Memory

- hq-tpays cold-start test completed (TEST_ID: coldstart-20260124-132053).
- Results: external PASSING (6/7), web_ui probe failed; internal NO_RESPONSE from canary mayor.
- Report: /home/shisui/gt/logs/coldstart-tests/coldstart-20260124-132053.json (latest.json updated).
- Mayor notified: COLDSTART_TEST issues + status update sent.
- Follow-up issues filed:
  - hq-t00sx: Investigate web_ui probe failure (no :8080 listener).
  - hq-ifo2u: Investigate mayor NO_RESPONSE to COLDSTART_PROBE.
- deploy/canary-manifest.yaml removal already in repo history (commit c874f77b).
