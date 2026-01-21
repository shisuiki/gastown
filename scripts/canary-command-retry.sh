#!/usr/bin/env bash
set -euo pipefail

COMMAND=""
CONTAINER=""
TIMEOUT=""
ATTEMPTS=""
BACKOFF=""
NO_RETRY=false
CONTEXT_FILE="${CANARY_FAILURE_CONTEXT_FILE:-canary-failure-context.txt}"
WHITELIST=""
BLACKLIST=""

usage() {
  cat <<USAGE
Usage: $0 --command <name> [--container <name>] [--timeout <seconds>] [--attempts <n>] [--backoff <seconds>] [--no-retry] [--whitelist <list>] [--blacklist <list>]

Options:
  -c, --command     Command name (health, config, migrate, smoke).
  -n, --container   Override CANARY_CONTAINER for the inner docker exec call.
  -t, --timeout     Override CANARY_COMMAND_TIMEOUT for the inner call.
  -a, --attempts    Override CANARY_RETRY_ATTEMPTS (defaults to env or 3).
  -b, --backoff     Override CANARY_RETRY_BACKOFF_SECONDS (defaults to env or 10).
      --whitelist   Comma-separated list of commands that may be retried (default: all).
      --blacklist   Comma-separated list of commands that must never retry.
      --no-retry    Disable retries for this run (single attempt only).
  -h, --help        Show this message.
USAGE
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    -c|--command)
      shift
      COMMAND="$1"
      ;;
    -n|--container)
      shift
      CONTAINER="$1"
      ;;
    -t|--timeout)
      shift
      TIMEOUT="$1"
      ;;
    -a|--attempts)
      shift
      ATTEMPTS="$1"
      ;;
    -b|--backoff)
      shift
      BACKOFF="$1"
      ;;
    --whitelist)
      shift
      WHITELIST="$1"
      ;;
    --blacklist)
      shift
      BLACKLIST="$1"
      ;;
    --no-retry)
      NO_RETRY=true
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

if [[ -z "$COMMAND" ]]; then
  echo "--command is required" >&2
  usage
  exit 2
fi

ATTEMPTS="${ATTEMPTS:-${CANARY_RETRY_ATTEMPTS:-3}}"
BACKOFF="${BACKOFF:-${CANARY_RETRY_BACKOFF_SECONDS:-10}}"
if ! [[ "$ATTEMPTS" =~ ^[0-9]+$ ]] || [[ "$ATTEMPTS" -lt 1 ]]; then
  echo "Invalid --attempts value" >&2
  exit 2
fi
if ! [[ "$BACKOFF" =~ ^[0-9]+$ ]] || [[ "$BACKOFF" -lt 0 ]]; then
  echo "Invalid --backoff value" >&2
  exit 2
fi

IFS=',' read -ra whitelist_array <<< "${WHITELIST}"
IFS=',' read -ra blacklist_array <<< "${BLACKLIST}"

contains() {
  local needle="$1"
  shift
  local candidate
  for candidate in "$@"; do
    candidate="${candidate// /}"
    if [[ -n "$candidate" && "$candidate" == "$needle" ]]; then
      return 0
    fi
  done
  return 1
}

retry_allowed=true
if [[ "$NO_RETRY" == true ]]; then
  retry_allowed=false
elif [[ -n "$BLACKLIST" ]] && contains "$COMMAND" "${blacklist_array[@]}"; then
  retry_allowed=false
elif [[ -n "$WHITELIST" ]] && ! contains "$COMMAND" "${whitelist_array[@]}"; then
  retry_allowed=false
fi

write_context() {
  local status="$1"
  local message="$2"
  local exit_code="$3"
  local attempts="$4"
  local delay="$5"
  cat <<EOF > "$CONTEXT_FILE"
status=$status
command=$COMMAND
exit_code=$exit_code
attempts=$attempts
retry_allowed=$retry_allowed
last_delay_seconds=$delay
message=$message
EOF
}

run_once() {
  local args=("scripts/canary-docker-exec.sh" --command "$COMMAND")
  [[ -n "$CONTAINER" ]] && args+=(--container "$CONTAINER")
  [[ -n "$TIMEOUT" ]] && args+=(--timeout "$TIMEOUT")
  "${args[@]}"
}

if [[ "$retry_allowed" == false ]]; then
  write_context pending "Retries disabled for $COMMAND" 0 0 0
  run_once
  write_context success "$COMMAND succeeded" 0 1 0
  exit 0
fi

attempt=0
while [[ $attempt -lt $ATTEMPTS ]]; do
  attempt=$((attempt + 1))
  if run_once; then
    write_context success "$COMMAND succeeded on attempt $attempt" 0 $attempt 0
    exit 0
  fi
  rc=$?
  message="Attempt $attempt failed (exit $rc)"
  if [[ $attempt -lt $ATTEMPTS ]]; then
    delay=$((BACKOFF * (2 ** (attempt - 1))))
    message+="; retrying in ${delay}s"
    echo "$message" >&2
    write_context retrying "$message" $rc $attempt $delay
    sleep "$delay"
  else
    echo "$message" >&2
    write_context failure "$message" $rc $attempt 0
    exit $rc
  fi
done
