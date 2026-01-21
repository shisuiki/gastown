#!/bin/bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

CANARY_DEPLOY_ROOT="${CANARY_DEPLOY_ROOT:-$HOME/gt/deployments/canary}"
ENV_CONFIG_REPO="${ENV_CONFIG_REPO:-$HOME/gt/env-config}"

GASTOWN_SHA="${GASTOWN_SHA:-}"
ENV_CONFIG_SHA="${ENV_CONFIG_SHA:-}"
IMAGE_TAG="${IMAGE_TAG:-}"
DEPLOY_RESULT="${DEPLOY_RESULT:-pending}"

usage() {
	cat <<'USAGE'
Usage: compose.sh --gastown-sha <sha> --env-config-sha <sha> --image-tag <tag> [options]

Options:
  --result <status>      Deploy result for manifest (default: pending)
  --env-config-repo <p>  Path to env-config repo (default: $HOME/gt/env-config)
  --deploy-root <p>      Base dir for deployment artifacts (default: $HOME/gt/deployments/canary)

Env:
  GASTOWN_SHA, ENV_CONFIG_SHA, IMAGE_TAG, DEPLOY_RESULT
  ENV_CONFIG_REPO, CANARY_DEPLOY_ROOT
USAGE
}

while [ $# -gt 0 ]; do
	case "$1" in
		--gastown-sha) GASTOWN_SHA="$2"; shift 2 ;;
		--env-config-sha) ENV_CONFIG_SHA="$2"; shift 2 ;;
		--image-tag) IMAGE_TAG="$2"; shift 2 ;;
		--result) DEPLOY_RESULT="$2"; shift 2 ;;
		--env-config-repo) ENV_CONFIG_REPO="$2"; shift 2 ;;
		--deploy-root) CANARY_DEPLOY_ROOT="$2"; shift 2 ;;
		-h|--help) usage; exit 0 ;;
		*) echo "Unknown option: $1" >&2; usage; exit 2 ;;
	esac
done

if [ -z "$GASTOWN_SHA" ] || [ -z "$ENV_CONFIG_SHA" ] || [ -z "$IMAGE_TAG" ]; then
	echo "ERROR: --gastown-sha, --env-config-sha, and --image-tag are required" >&2
	exit 2
fi

if ! git -C "$ENV_CONFIG_REPO" rev-parse --is-inside-work-tree >/dev/null 2>&1; then
	echo "ERROR: env-config repo not found at $ENV_CONFIG_REPO" >&2
	exit 1
fi

mkdir -p "$CANARY_DEPLOY_ROOT/env-config"
ENV_CONFIG_PATH="${CANARY_DEPLOY_ROOT}/env-config/${ENV_CONFIG_SHA}"

if [ ! -d "$ENV_CONFIG_PATH/.git" ]; then
	git -C "$ENV_CONFIG_REPO" fetch --all --quiet
	git -C "$ENV_CONFIG_REPO" worktree add --detach "$ENV_CONFIG_PATH" "$ENV_CONFIG_SHA"
fi

COMPAT_FILE="${ENV_CONFIG_COMPAT_FILE:-$ENV_CONFIG_PATH/compat.json}"
if [ -f "$COMPAT_FILE" ] && command -v jq >/dev/null 2>&1; then
	COMPAT_GASTOWN_SHA="$(jq -r '.gastown_sha // empty' "$COMPAT_FILE")"
	if [ -n "$COMPAT_GASTOWN_SHA" ] && [ "$COMPAT_GASTOWN_SHA" != "$GASTOWN_SHA" ]; then
		echo "ERROR: env-config expects gastown_sha ${COMPAT_GASTOWN_SHA}, got ${GASTOWN_SHA}" >&2
		exit 3
	fi
fi

MANIFEST_PATH="$("${SCRIPT_DIR}/manifest.sh" record \
	--gastown-sha "$GASTOWN_SHA" \
	--env-config-sha "$ENV_CONFIG_SHA" \
	--image-tag "$IMAGE_TAG" \
	--result "$DEPLOY_RESULT")"

ENV_FILE="${CANARY_DEPLOY_ROOT}/last-deploy.env"
cat >"$ENV_FILE" <<EOF
GASTOWN_SHA=${GASTOWN_SHA}
ENV_CONFIG_SHA=${ENV_CONFIG_SHA}
IMAGE_TAG=${IMAGE_TAG}
ENV_CONFIG_PATH=${ENV_CONFIG_PATH}
MANIFEST_PATH=${MANIFEST_PATH}
EOF

echo "Env-config worktree: ${ENV_CONFIG_PATH}"
echo "Deploy manifest: ${MANIFEST_PATH}"
echo "Deploy env file: ${ENV_FILE}"
