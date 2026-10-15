#!/usr/bin/env bash
#
# verify-sleep-live.sh: live check of Sleep when idle against a real Docker
# daemon, a real control plane and the real embedded ingress. Every step
# asserts the container state with `docker ps`, not just the API's word.
#
# Requires: Go, Docker, curl, jq. Uses a scratch data dir and ports
# 18098 (API), 18080/18443 (ingress); removes everything on exit.
set -uo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SCRATCH="$(mktemp -d "${TMPDIR:-/tmp}/levelrail-sleep.XXXXXX")"
BIN="$SCRATCH/levelrail"
API="http://127.0.0.1:18098"
TOKEN="dev-root-token"
APP="sleep-verify"
HOST="sleep-verify.example.test"
PID=""
PASS=0

log() { echo "[verify-sleep] $*"; }
fail() {
  log "FAIL: $*"
  docker ps -a --filter "name=${APP}" --format '[verify-sleep] container {{.Names}} {{.Status}}'
  grep -E 'sleep|application/' "$SCRATCH/server.log" 2>/dev/null | tail -12 | cut -c1-240
  exit 1
}
ok() { PASS=$((PASS + 1)); log "ok: $*"; }

cleanup() {
  [ -n "$PID" ] && kill "$PID" 2>/dev/null && wait "$PID" 2>/dev/null
  for c in $(docker ps -aq --filter "name=${APP}"); do docker rm -f "$c" >/dev/null 2>&1; done
  rm -rf "$SCRATCH"
}
trap cleanup EXIT

api() { # method path [json]
  curl -s -o "$SCRATCH/body" -w '%{http_code}' -X "$1" -H "Authorization: Bearer $TOKEN" \
    -H 'Content-Type: application/json' ${3:+-d "$3"} "$API$2"
}
running() { docker ps --filter "name=${APP}" --filter status=running -q | grep -q .; }
wait_for() { # description timeout cmd...
  local d="$1" n="$2" i=0; shift 2
  until "$@"; do i=$((i + 1)); [ "$i" -ge "$n" ] && fail "timed out: $d"; sleep 1; done
}
stopped() { ! running; }
getfollow() { curl -sk --max-time 90 -L --resolve "$HOST:18443:127.0.0.1" -o "$SCRATCH/page" -w '%{http_code} %{num_redirects}' "https://$HOST:18443/"; }
get() { curl -sk --max-time 60 --resolve "$HOST:18443:127.0.0.1" -o "$SCRATCH/page" -w '%{http_code}' "https://$HOST:18443/"; }

docker info >/dev/null 2>&1 || fail "docker daemon not reachable"
docker pull -q nginx:alpine >/dev/null || fail "cannot pull nginx:alpine"

log "building control plane"
(cd "$REPO_ROOT" && go build -o "$BIN" ./cmd/levelrail) || fail "build failed"

log "starting control plane"
(cd "$REPO_ROOT" && exec env APP_DEV_MODE=1 APP_ADMIN_USERNAME=dev APP_ADMIN_PASSWORD=dev \
  APP_DATA_DIR="$SCRATCH/data" APP_HTTP_ADDR=":18098" APP_AGENT_ADDR=":19543" \
  APP_INGRESS_HTTP_ADDR=":18080" APP_INGRESS_HTTPS_ADDR=":18443" \
  APP_MESH_ENABLED=1 APP_AUTO_PLACEMENT=false "$BIN" >"$SCRATCH/server.log" 2>&1) &
PID=$!
wait_for "control plane healthy" 40 curl -sf -o /dev/null "$API/api/v1/brand"

log "creating $APP"
[ "$(api POST /api/v1/apps "{\"name\":\"$APP\",\"image\":\"nginx:alpine\",\"port\":80}")" = "201" ] \
  || [ "$(api GET /api/v1/apps/$APP)" = "200" ] || fail "create app: $(cat "$SCRATCH/body")"
wait_for "container running" 60 running
ok "app running"

code="$(api POST /api/v1/apps/$APP/sleep/now)"
[ "$code" = "409" ] || fail "sleep now with sleep off returned $code, want 409"
ok "sleep now refused while sleep is off"

log "attaching a domain"
code="$(api PUT /api/v1/apps/$APP "{\"image\":\"nginx:alpine\",\"port\":80,\"domains\":[\"$HOST\"]}")"
[ "$code" = "200" ] || fail "set domain returned $code: $(cat "$SCRATCH/body")"
wait_for "ingress serves the app" 60 bash -c "$(declare -f get); SCRATCH='$SCRATCH' HOST='$HOST'; [ \"\$(get)\" = 200 ]"
ok "request through the ingress returns 200"

code="$(api PUT /api/v1/apps/$APP/sleep '{"idle_minutes":30}')"
[ "$code" = "200" ] || fail "enable sleep returned $code: $(cat "$SCRATCH/body")"
ok "sleep enabled"

code="$(api POST /api/v1/apps/$APP/sleep/now)"
[ "$code" = "200" ] || fail "sleep now returned $code: $(cat "$SCRATCH/body")"
wait_for "container stopped" 90 stopped
[ "$(jq -r .sleeping "$SCRATCH/body")" = "true" ] || fail "API did not report sleeping"
ok "sleep now stopped the container"
[ "$(api GET /api/v1/sleep)" = "200" ] && [ "$(jq -r .sleeping "$SCRATCH/body")" = "1" ] \
  || fail "fleet endpoint does not show 1 sleeping: $(cat "$SCRATCH/body")"
ok "fleet endpoint counts the sleeping app"

log "first request while asleep (default mode: waking page, then retry)"
first="$(get)"
log "first response: $first"
wait_for "app woke on request" 90 running
wait_for "app answers 200 after wake" 60 bash -c "$(declare -f get); SCRATCH='$SCRATCH' HOST='$HOST'; [ \"\$(get)\" = 200 ]"
api GET /api/v1/apps/$APP/sleep >/dev/null
[ "$(jq -r .sleeping "$SCRATCH/body")" = "false" ] || fail "sleeping flag survived the wake"
ok "request woke the app and it serves 200 again"

log "hold mode: the very request that wakes the app is replayed"
api PUT /api/v1/apps/$APP/sleep '{"idle_minutes":30,"hold_requests":true}' >/dev/null
[ "$(api POST /api/v1/apps/$APP/sleep/now)" = "200" ] || fail "sleep now (hold) failed"
wait_for "container stopped (hold)" 40 stopped
held="$(getfollow)"
[ "$held" = "200 1" ] || fail "held request returned '$held', want '200 1' (one redirect back to the same URL, then the app)"
running || fail "app not running after held request"
ok "held request answered once the app was ready: one redirect, then 200"

log "manual wake clears the state"
api POST /api/v1/apps/$APP/sleep/now >/dev/null
wait_for "container stopped (manual)" 40 stopped
[ "$(api POST /api/v1/apps/$APP/sleep/wake)" = "200" ] || fail "wake failed"
wait_for "container running (manual)" 60 running
ok "wake now restarted the app"

log "turning sleep off"
[ "$(api PUT /api/v1/apps/$APP/sleep '{"idle_minutes":0}')" = "200" ] || fail "disable failed"
[ "$(api GET /api/v1/sleep)" = "200" ] && [ "$(jq -r .enabled "$SCRATCH/body")" = "0" ] || fail "fleet still lists the app"
ok "sleep disabled"

log "ALL $PASS CHECKS PASSED"
