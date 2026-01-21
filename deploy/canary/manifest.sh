#!/bin/bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

CANARY_DEPLOY_ROOT="${CANARY_DEPLOY_ROOT:-$HOME/gt/deployments/canary}"
MANIFEST_DIR="${CANARY_DEPLOY_ROOT}/manifests"
RETENTION_COUNT="${CANARY_DEPLOY_RETENTION:-10}"

usage() {
	cat <<'USAGE'
Usage: manifest.sh <command> [options]

Commands:
  record   --gastown-sha <sha> --env-config-sha <sha> --image-tag <tag> [--result <status>]
  list     [--limit <n>]
  show     <manifest-path>
  latest

Env:
  CANARY_DEPLOY_ROOT      Base dir for manifests (default: $HOME/gt/deployments/canary)
  CANARY_DEPLOY_RETENTION Number of manifests to retain (default: 10)
USAGE
}

record_manifest() {
	local gastown_sha=""
	local env_config_sha=""
	local image_tag=""
	local deploy_result="pending"

	while [ $# -gt 0 ]; do
		case "$1" in
			--gastown-sha) gastown_sha="$2"; shift 2 ;;
			--env-config-sha) env_config_sha="$2"; shift 2 ;;
			--image-tag) image_tag="$2"; shift 2 ;;
			--result) deploy_result="$2"; shift 2 ;;
			*) echo "Unknown option: $1" >&2; usage; exit 2 ;;
		esac
	done

	if [ -z "$gastown_sha" ] || [ -z "$env_config_sha" ] || [ -z "$image_tag" ]; then
		echo "ERROR: --gastown-sha, --env-config-sha, and --image-tag are required" >&2
		exit 2
	fi

	mkdir -p "$MANIFEST_DIR"
	local timestamp
	timestamp="$(date -u +"%Y-%m-%dT%H:%M:%SZ")"
	local manifest_path="${MANIFEST_DIR}/deploy-${timestamp}.json"

	cat >"$manifest_path" <<EOF
{
  "timestamp": "${timestamp}",
  "gastown_sha": "${gastown_sha}",
  "env_config_sha": "${env_config_sha}",
  "image_tag": "${image_tag}",
  "deploy_result": "${deploy_result}"
}
EOF

	prune_manifests
	echo "$manifest_path"
}

list_manifests() {
	local limit=10
	if [ "${1:-}" = "--limit" ]; then
		limit="${2:-10}"
		shift 2
	fi

	if [ ! -d "$MANIFEST_DIR" ]; then
		echo "No manifests found at $MANIFEST_DIR"
		return 0
	fi

	local manifests
	manifests="$(ls -t "$MANIFEST_DIR"/deploy-*.json 2>/dev/null | head -n "$limit" || true)"
	if [ -z "$manifests" ]; then
		echo "No manifests found at $MANIFEST_DIR"
		return 0
	fi

	if command -v jq >/dev/null 2>&1; then
		echo "$manifests" | while read -r path; do
			jq -r --arg path "$path" '"\(.timestamp) \(.gastown_sha) \(.env_config_sha) \(.image_tag) \(.deploy_result) \($path)"' "$path"
		done
	else
		echo "$manifests"
	fi
}

show_manifest() {
	local path="${1:-}"
	if [ -z "$path" ]; then
		echo "ERROR: manifest path required" >&2
		exit 2
	fi
	cat "$path"
}

latest_manifest() {
	if [ ! -d "$MANIFEST_DIR" ]; then
		return 1
	fi
	ls -t "$MANIFEST_DIR"/deploy-*.json 2>/dev/null | head -n 1
}

prune_manifests() {
	if [ ! -d "$MANIFEST_DIR" ]; then
		return 0
	fi

	local count
	count="$(ls -1 "$MANIFEST_DIR"/deploy-*.json 2>/dev/null | wc -l | tr -d ' ')"
	if [ "$count" -le "$RETENTION_COUNT" ]; then
		return 0
	fi

	ls -t "$MANIFEST_DIR"/deploy-*.json | tail -n +"$((RETENTION_COUNT + 1))" | xargs -r rm -f
}

cmd="${1:-}"
shift || true

case "$cmd" in
	record) record_manifest "$@" ;;
	list) list_manifests "$@" ;;
	show) show_manifest "$@" ;;
	latest) latest_manifest ;;
	*) usage; exit 2 ;;
esac
