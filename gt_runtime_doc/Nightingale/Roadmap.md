# Roadmap

## hq-tpays: Cold-start test + CI/CD status

### Phase 1: Prep
- Confirm prerequisite completion (hq-xe36v)
- Review cold-start procedure and locate CI/CD status data source

Acceptance
- Procedure understood and target files located

### Phase 2: Execute test
- Run mol-canary-coldstart-test (or manual procedure)
- Capture timing + health results

Acceptance
- New test artifact written under /home/shisui/gt/logs/coldstart-tests/

### Phase 3: Report + docs
- Update CI/CD status panel data with latest results
- Append test history entry in coldstart-procedure.md
- Communicate status to mayor (per template)

Acceptance
- Data updated and docs reflect new test run
