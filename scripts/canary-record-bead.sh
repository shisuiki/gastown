#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR=$(git rev-parse --show-toplevel 2>/dev/null || pwd)
GT_ROOT=${GT_ROOT:-/home/shisui/gt}
STATE_DIR=${CANARY_STATE_DIR:-"$GT_ROOT/logs"}
STATE_JSON=${CANARY_STATE_JSON:-"$STATE_DIR/canary-deploy.json"}
STATE_ENV=${CANARY_STATE_ENV:-"$STATE_DIR/canary-deploy.env"}
CONTAINER_NAME=${CONTAINER_NAME:-${CANARY_CONTAINER:-gastown-canary}}
CANARY_PORT=${CANARY_PORT:-8081}
CANARY_RESULT=${CANARY_RESULT:-success}
CANARY_RECORD_BEAD=${CANARY_RECORD_BEAD:-1}
PARENT_EPIC=${CANARY_PARENT_EPIC:-hq-vsa3v}

log() {
  printf "[%s] %s\n" "$(date -u +%Y-%m-%dT%H:%M:%SZ)" "$*"
}

if [ "$CANARY_RECORD_BEAD" = "0" ]; then
  log "CANARY_RECORD_BEAD=0; skipping deploy bead"
  exit 0
fi

if ! command -v bd >/dev/null 2>&1; then
  log "bd not found; skipping deploy bead"
  exit 0
fi

if [ -f "$STATE_ENV" ]; then
  # shellcheck disable=SC1090
  source "$STATE_ENV"
fi

read_json_field() {
  local field=$1
  if [ -f "$STATE_JSON" ]; then
    if command -v python3 >/dev/null 2>&1; then
      python3 - "$STATE_JSON" "$field" <<'PY'
import json
import sys

path = sys.argv[1]
field = sys.argv[2]
try:
    with open(path, "r", encoding="utf-8") as handle:
        data = json.load(handle)
except Exception:
    sys.exit(0)

value = data.get(field)
if value is None:
    sys.exit(0)
print(value)
PY
      return 0
    fi

    if command -v jq >/dev/null 2>&1; then
      jq -r --arg field "$field" '.[$field] // empty' "$STATE_JSON"
      return 0
    fi
  fi
}

GASTOWN_REF=${GASTOWN_REF:-}
ENV_CONFIG_REF=${ENV_CONFIG_REF:-}
IMAGE_TAG=${IMAGE_TAG:-${CURRENT_IMAGE:-}}
HOSTNAME_VALUE=$(hostname 2>/dev/null || echo "unknown")

if [ -z "$GASTOWN_REF" ]; then
  GASTOWN_REF=$(read_json_field "gastown_ref" || true)
fi

if [ -z "$ENV_CONFIG_REF" ]; then
  ENV_CONFIG_REF=$(read_json_field "env_config_ref" || true)
fi

if [ -z "$IMAGE_TAG" ]; then
  IMAGE_TAG=$(read_json_field "image" || true)
fi

CANARY_TIMESTAMP=$(date -u +%Y-%m-%dT%H:%M:%SZ)

if command -v python3 >/dev/null 2>&1; then
  payload=$(python3 - <<'PY'
import json
import os

payload = {
    "timestamp": os.environ.get("CANARY_TIMESTAMP"),
    "gastown_ref": os.environ.get("GASTOWN_REF") or "unknown",
    "env_config_ref": os.environ.get("ENV_CONFIG_REF") or "unknown",
    "image_tag": os.environ.get("IMAGE_TAG") or "unknown",
    "result": os.environ.get("CANARY_RESULT") or "unknown",
    "container": os.environ.get("CONTAINER_NAME") or "gastown-canary",
    "canary_port": os.environ.get("CANARY_PORT") or "8081",
    "state_json": os.environ.get("STATE_JSON") or "",
    "state_env": os.environ.get("STATE_ENV") or "",
    "host": os.environ.get("HOSTNAME_VALUE") or "unknown",
}

print(json.dumps(payload, separators=(",", ":"), sort_keys=True))
PY
  )
else
  payload=$(printf '{"timestamp":"%s","gastown_ref":"%s","env_config_ref":"%s","image_tag":"%s","result":"%s","container":"%s","canary_port":"%s","state_json":"%s","state_env":"%s","host":"%s"}' \
    "$CANARY_TIMESTAMP" \
    "${GASTOWN_REF:-unknown}" \
    "${ENV_CONFIG_REF:-unknown}" \
    "${IMAGE_TAG:-unknown}" \
    "${CANARY_RESULT:-unknown}" \
    "${CONTAINER_NAME:-gastown-canary}" \
    "${CANARY_PORT:-8081}" \
    "${STATE_JSON:-}" \
    "${STATE_ENV:-}" \
    "$HOSTNAME_VALUE")
fi

short_ref=""
if [ -n "$GASTOWN_REF" ]; then
  short_ref="${GASTOWN_REF:0:12}"
fi

title="Canary deploy ${CANARY_RESULT}"
if [ -n "$short_ref" ]; then
  title="$title ${short_ref}"
fi

cmd=(bd create --type=event --title "$title" --event-category=canary.deploy --event-target="container:${CONTAINER_NAME}" --event-payload="$payload")
if [ -n "$PARENT_EPIC" ]; then
  cmd+=(--parent "$PARENT_EPIC")
fi

"${cmd[@]}"
log "Recorded deploy bead for $CONTAINER_NAME ($CANARY_RESULT)"
