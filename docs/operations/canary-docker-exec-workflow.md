# Canary Docker Exec Workflow

This workflow documents how to validate the canary container locally or in a staging host.

## Build and Run

```bash
docker build -t gastown:canary .

# Start the container

docker run -d --name gastown-canary \
  -p 8080:8080 \
  -v /path/to/gt-workspace:/gt \
  -e GT_WEB_AUTH_TOKEN=changeme \
  -e GT_WEB_ALLOW_REMOTE=1 \
  gastown:canary
```

## Validate Binaries

```bash
docker exec gastown-canary gt version
docker exec gastown-canary bd version
```

## Logs and Health

```bash
docker logs -f gastown-canary

# HTTP health check
curl -H "Authorization: Bearer $GT_WEB_AUTH_TOKEN" http://localhost:8080/api/version
```

## Cleanup

```bash
docker stop gastown-canary
docker rm gastown-canary
```
