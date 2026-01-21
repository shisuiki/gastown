#!/usr/bin/env bash
set -euo pipefail

CONTEXT_FILE="${CANARY_FAILURE_CONTEXT_FILE:-canary-failure-context.txt}"
ALERT_EMAIL="${CANARY_ALERT_EMAIL:-}"
ALERT_WEBHOOK="${CANARY_ALERT_WEBHOOK:-}"
ALERT_LEVEL="${CANARY_ALERT_LEVEL:-warning}"
ALERT_MESSAGE="${CANARY_ALERT_MESSAGE:-Canary deployment failed.}"
ALERT_SOURCE="${CANARY_ALERT_SOURCE:-canary deployment workflow}"

usage() {
  cat <<USAGE
Usage: $0 [--context <path>] [--level <level>] [--message <text>] [--source <identifier>]

Options:
  --context   Path to failure context file (defaults to $CONTEXT_FILE).
  --level     Notification level (warning, critical, info).
  --message   Override the summary text.
  --source    Override the alert source shown in the payload.
  -h, --help  Show this message.
USAGE
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --context)
      shift
      CONTEXT_FILE="$1"
      ;;
    --level)
      shift
      ALERT_LEVEL="$1"
      ;;
    --message)
      shift
      ALERT_MESSAGE="$1"
      ;;
    --source)
      shift
      ALERT_SOURCE="$1"
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

context_summary="(no context available)"
if [[ -f "$CONTEXT_FILE" ]]; then
  context_summary=$(tr '\n' ' ' < "$CONTEXT_FILE" | sed -E 's/ +/ /g' | sed -E 's/^ | $//g')
fi

summary="[$ALERT_LEVEL] $ALERT_SOURCE: $ALERT_MESSAGE"
echo "$summary"
echo "Context: $context_summary"

payload=$(
  cat <<PAYLOAD
{
  "level": "${ALERT_LEVEL}",
  "source": "${ALERT_SOURCE}",
  "message": "${ALERT_MESSAGE}",
  "context": "${context_summary}"
}
PAYLOAD
)

if [[ -n "$ALERT_WEBHOOK" ]]; then
  if command -v curl >/dev/null 2>&1; then
    if ! curl -fsS -X POST -H "Content-Type: application/json" -d "$payload" "$ALERT_WEBHOOK"; then
      echo "Failed to POST alert payload to webhook" >&2
    fi
  else
    echo "curl not available; webhook alert skipped" >&2
  fi
fi

if [[ -n "$ALERT_EMAIL" ]]; then
  if command -v mail >/dev/null 2>&1; then
    printf "%s\n\n%s\n" "$summary" "$context_summary" | mail -s "$summary" "$ALERT_EMAIL" || \
      echo "Failed to send mail alert to $ALERT_EMAIL" >&2
  else
    echo "mail command unavailable; email alert skipped" >&2
  fi
fi
