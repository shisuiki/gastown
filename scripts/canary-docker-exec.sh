#!/usr/bin/env bash
set -euo pipefail

readonly DEFAULT_TIMEOUT_SECONDS=300

usage() {
  cat <<USAGE
Usage: $0 [options]

Options:
  -c, --command <name>   One of health, config, migrate, smoke, rollback.
  -n, --container <name> Override CANARY_CONTAINER (default: gastown-canary).
  -t, --timeout <sec>    Override CANARY_COMMAND_TIMEOUT (default: ${DEFAULT_TIMEOUT_SECONDS}).
  -h, --help             Show this help message.
USAGE
}

COMMAND_NAME=""
CONTAINER=""
OVERRIDE_TIMEOUT=""

while [[ $# -gt 0 ]]; do
  case "$1" in
    -c|--command)
      if [[ $# -lt 2 ]]; then
        echo "Missing value for $1" >&2
        usage
        exit 2
      fi
      shift
      COMMAND_NAME="$1"
      ;;
    -n|--container)
      if [[ $# -lt 2 ]]; then
        echo "Missing value for $1" >&2
        usage
        exit 2
      fi
      shift
      CONTAINER="$1"
      ;;
    -t|--timeout)
      if [[ $# -lt 2 ]]; then
        echo "Missing value for $1" >&2
        usage
        exit 2
      fi
      shift
      OVERRIDE_TIMEOUT="$1"
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "Unknown option: $1" >&2
      usage
      exit 2
      ;;
  esac
  shift
done

if [[ -z "$COMMAND_NAME" ]]; then
  echo "--command is required" >&2
  usage
  exit 2
fi

CONTAINER="${CONTAINER:-${CANARY_CONTAINER:-gastown-canary}}"
if [[ -z "$CONTAINER" ]]; then
  echo "Container name cannot be empty" >&2
  exit 2
fi

COMMAND_TIMEOUT_SECONDS="${OVERRIDE_TIMEOUT:-${CANARY_COMMAND_TIMEOUT:-$DEFAULT_TIMEOUT_SECONDS}}"
if ! [[ "$COMMAND_TIMEOUT_SECONDS" =~ ^[0-9]+$ ]]; then
  echo "Timeout must be a positive integer" >&2
  exit 2
fi

if ! command -v docker >/dev/null 2>&1; then
  echo "docker CLI is required" >&2
  exit 2
fi

trim_whitespace() {
  local value="$1"
  value="${value#${value%%[![:space:]]*}}"
  value="${value%${value##*[![:space:]]}}"
  printf '%s' "$value"
}

ensure_container_running() {
  if docker inspect --format '{{.State.Running}}' "$CONTAINER" >/dev/null 2>&1; then
    return 0
  fi

  echo "Container '$CONTAINER' is not running; attempting to start..." >&2
  if ! docker start "$CONTAINER" >/dev/null 2>&1; then
    echo "Failed to start container '$CONTAINER'." >&2
    return 1
  fi
  echo "Container '$CONTAINER' started." >&2
}

run_inner_command() {
  local inner_command="$1"
  printf '→ [%s] docker exec %s sh -c "%s"\n' "$COMMAND_NAME" "$CONTAINER" "$inner_command"
  local exit_code=0
  set +e
  if command -v timeout >/dev/null 2>&1; then
    timeout --preserve-status "$COMMAND_TIMEOUT_SECONDS" docker exec "$CONTAINER" sh -c "$inner_command"
    exit_code=$?
  else
    docker exec "$CONTAINER" sh -c "$inner_command"
    exit_code=$?
  fi
  set -e

  if [[ $exit_code -ne 0 ]]; then
    echo "Command '$inner_command' failed with exit code $exit_code" >&2
    return $exit_code
  fi
  return 0
}

declare -a INNER_COMMANDS=()

case "$COMMAND_NAME" in
  health)
    INNER_COMMANDS=("curl -fsS http://localhost:12121/_/health")
    ;;
  config)
    INNER_COMMANDS=("gt config reload")
    ;;
  migrate)
    INNER_COMMANDS=("gt migrations apply")
    ;;
  smoke)
    INNER_COMMANDS=("curl -fsS http://localhost:41241/_/ping")
    ;;
  rollback)
    if [[ -n "${CANARY_ROLLBACK_COMMANDS:-}" ]]; then
      IFS=';;' read -ra raw_commands <<< "${CANARY_ROLLBACK_COMMANDS}"
      for candidate in "${raw_commands[@]}"; do
        candidate="$(trim_whitespace "$candidate")"
        if [[ -n "$candidate" ]]; then
          INNER_COMMANDS+=("$candidate")
        fi
      done
    fi
    if [[ ${#INNER_COMMANDS[@]} -eq 0 ]]; then
      INNER_COMMANDS=("echo 'Rollback hook ran (no commands configured).'" )
    fi
    ;;
  *)
    echo "Unknown command: $COMMAND_NAME" >&2
    usage
    exit 2
    ;;
esac

if ! ensure_container_running; then
  exit 1
fi

for inner in "${INNER_COMMANDS[@]}"; do
  if ! run_inner_command "$inner"; then
    exit $?
  fi
done
