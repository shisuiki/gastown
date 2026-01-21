#!/usr/bin/env bash
set -euo pipefail

rig_name="${NIGHTINGALE_RIG:-nightingale}"
crew_name="${NIGHTINGALE_CREW:-Nightingale}"
target="${NIGHTINGALE_TARGET:-${rig_name}/crew/${crew_name}}"
priority="${NIGHTINGALE_PRIORITY:-1}"
reason="${NIGHTINGALE_REASON:-manual}"

if ! command -v bd >/dev/null 2>&1; then
  echo "bd is required in PATH" >&2
  exit 1
fi

if ! command -v gt >/dev/null 2>&1; then
  echo "gt is required in PATH" >&2
  exit 1
fi

now_utc="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
run_url=""
if [[ -n "${GITHUB_RUN_ID:-}" && -n "${GITHUB_REPOSITORY:-}" ]]; then
  run_url="https://github.com/${GITHUB_REPOSITORY}/actions/runs/${GITHUB_RUN_ID}"
fi

read -r -d '' description <<EOF_DESC || true
Trigger: ${GITHUB_EVENT_NAME:-manual}
Reason: ${reason}
Ref: ${GITHUB_REF_NAME:-}
SHA: ${GITHUB_SHA:-}
Run: ${run_url}
Time: ${now_utc}

Ops:
- docs/operations/nightingale-ops.md
- docs/operations/canary-docker-exec-workflow.md
- docs/CANARY-DEPLOY.md
- docs/MAYOR-CREW-DEPLOY.md
EOF_DESC

bead_id="$(bd new --type task --priority "${priority}" \
  --title "Nightingale CI/CD trigger ${now_utc}" \
  --description "${description}" \
  --labels "nightingale,cicd" \
  --silent)"

if [[ -z "$bead_id" ]]; then
  echo "Failed to create bead" >&2
  exit 1
fi

gt sling "$bead_id" "$target" --message "CI/CD trigger from GitHub Actions"
