#!/usr/bin/env bash
# Sends one real test message per notification kind, to check the receiving
# side by hand. Kinds whose env vars are unset are skipped. Secrets are never printed.
#
# Env vars (all optional):
#   SMOKE_DISCORD_WEBHOOK_URL
#   SMOKE_SLACK_WEBHOOK_URL
#   SMOKE_TELEGRAM_BOT_TOKEN and SMOKE_TELEGRAM_CHAT_ID
#   SMOKE_NTFY_URL (full topic URL) and optional SMOKE_NTFY_TOKEN
set -u

MSG="Levelrail notify smoke test $(date -u +%Y-%m-%dT%H:%M:%SZ)"
FAILED=0
SENT=0

# The URL goes through a curl config on stdin so tokens in it never reach argv or logs.
post() {
  local url="$1"
  shift
  local code
  code=$(printf 'url = "%s"\n' "$url" | curl -sS -K - -o /dev/null -w '%{http_code}' \
    --max-time 15 "$@" 2>/dev/null) || code="000"
  [ "$code" -ge 200 ] 2>/dev/null && [ "$code" -lt 300 ] 2>/dev/null
}

report() {
  local kind="$1" ok="$2"
  SENT=$((SENT + 1))
  if [ "$ok" = 0 ]; then
    echo "pass  $kind"
  else
    echo "fail  $kind"
    FAILED=$((FAILED + 1))
  fi
}

skip() { echo "skip  $1 (env not set)"; }

if [ -n "${SMOKE_DISCORD_WEBHOOK_URL:-}" ]; then
  post "$SMOKE_DISCORD_WEBHOOK_URL" -X POST -H 'Content-Type: application/json' \
    --data "{\"content\":\"$MSG\"}"
  report discord $?
else
  skip discord
fi

if [ -n "${SMOKE_SLACK_WEBHOOK_URL:-}" ]; then
  post "$SMOKE_SLACK_WEBHOOK_URL" -X POST -H 'Content-Type: application/json' \
    --data "{\"text\":\"$MSG\"}"
  report slack $?
else
  skip slack
fi

if [ -n "${SMOKE_TELEGRAM_BOT_TOKEN:-}" ] && [ -n "${SMOKE_TELEGRAM_CHAT_ID:-}" ]; then
  post "https://api.telegram.org/bot${SMOKE_TELEGRAM_BOT_TOKEN}/sendMessage" -X POST \
    --data-urlencode "chat_id=${SMOKE_TELEGRAM_CHAT_ID}" --data-urlencode "text=$MSG"
  report telegram $?
else
  skip telegram
fi

if [ -n "${SMOKE_NTFY_URL:-}" ]; then
  ntfy_args=(-X POST --data "$MSG")
  if [ -n "${SMOKE_NTFY_TOKEN:-}" ]; then
    ntfy_args+=(-H "Authorization: Bearer ${SMOKE_NTFY_TOKEN}")
  fi
  post "$SMOKE_NTFY_URL" "${ntfy_args[@]}"
  report ntfy $?
else
  skip ntfy
fi

echo "sent $SENT, failed $FAILED"
[ "$FAILED" -eq 0 ]
