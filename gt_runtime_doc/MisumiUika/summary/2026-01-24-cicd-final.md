CI/CD WebUI delivery
- Added CI/CD status endpoints with cached GitHub Actions + log-backed reports.
- Integrated CI/CD summary into dashboard WebSocket payloads and added dashboard card.
- Built /cicd detail page with workflow list, run history, filters, and pagination.
- Documented endpoints/caching and validated with go test ./internal/web.
