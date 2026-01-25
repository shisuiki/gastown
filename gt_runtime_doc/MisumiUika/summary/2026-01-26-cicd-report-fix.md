CI/CD report rendering fix
- Parsed coldstart latest.json into separate external/internal reports.
- Added report list rendering so coldstart internal/external + canary show together.
- Tests: go test ./internal/web failed (TestGUIHandler_APISendMail timeout/false success).
