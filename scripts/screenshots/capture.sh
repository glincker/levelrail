#!/usr/bin/env bash
#
# capture.sh: end-to-end pipeline for docs/assets/screenshots/*.png (see
# docs/screenshots.md). Builds fresh binaries, runs a real control plane
# against a scratch data dir, deploys two real containers, logs in
# through the actual /login form, and drives shot-scraper against the
# real running dashboard. Safely re-runnable: everything lives under a
# throwaway scratch dir and gets torn down on exit, including on error
# (the EXIT trap runs regardless).
#
# Requires: Go, Node/npm (for web/dist), Docker, Python 3 with
# playwright installed, and shot-scraper on PATH (pip install
# shot-scraper && shot-scraper install).
set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"

HERO_APP="marketing-site"
SECOND_APP="edge-cache"

SCRATCH_DIR="$(mktemp -d "${TMPDIR:-/tmp}/levelrail-screenshot.XXXXXX")"
BIN="$SCRATCH_DIR/levelrail"
CLI="$SCRATCH_DIR/levelrail-cli"
DATA_DIR="$SCRATCH_DIR/data"
FAKE_HOME="$SCRATCH_DIR/home"
SERVER_LOG="$SCRATCH_DIR/server.log"
DEVICE_LOGIN_LOG="$SCRATCH_DIR/device-login.log"
STORAGE_STATE="$SCRATCH_DIR/storage_state.json"
SESSION_COOKIE_FILE="$SCRATCH_DIR/session_cookie.txt"
RENDERED_SHOTS="$SCRATCH_DIR/shots.rendered.yml"
mkdir -p "$DATA_DIR" "$FAKE_HOME"

SERVER_PID=""
DEVICE_LOGIN_PID=""

log() { echo "[capture.sh] $*"; }

cleanup() {
  export HOME="$FAKE_HOME"
  if [ -n "$SERVER_PID" ] && kill -0 "$SERVER_PID" 2>/dev/null; then
    if [ -f "$CLI" ]; then
      "$CLI" apps delete "$HERO_APP" >/dev/null 2>&1
      "$CLI" apps delete "$SECOND_APP" >/dev/null 2>&1
      i=0
      while docker ps --format '{{.Names}}' 2>/dev/null | grep -qE "^(${HERO_APP}|${SECOND_APP})-"; do
        i=$((i + 1))
        [ "$i" -ge 15 ] && break
        sleep 2
      done
    fi
    kill "$SERVER_PID" 2>/dev/null
    wait "$SERVER_PID" 2>/dev/null
  fi
  if [ -n "$DEVICE_LOGIN_PID" ] && kill -0 "$DEVICE_LOGIN_PID" 2>/dev/null; then
    kill "$DEVICE_LOGIN_PID" 2>/dev/null
  fi
  # Fallback: apps delete only tears a container down via the next
  # reconcile pass, which needs the server alive; anything the loop
  # above timed out on gets removed directly so a failed run never
  # leaks containers.
  for name in "$HERO_APP" "$SECOND_APP"; do
    docker ps -a --format '{{.Names}}' 2>/dev/null | grep -E "^${name}-" | xargs -r docker rm -f >/dev/null 2>&1
  done
  if [ "${KEEP_SCRATCH:-0}" != "1" ]; then
    rm -rf "$SCRATCH_DIR"
  else
    log "KEEP_SCRATCH=1: leaving $SCRATCH_DIR in place"
  fi
}
trap cleanup EXIT

fail() {
  log "ERROR: $*"
  exit 1
}

# --- pick free ports -----------------------------------------------------

port_free() {
  ! lsof -i ":$1" >/dev/null 2>&1
}

pick_port() {
  local start="$1" port
  for port in $(seq "$start" $((start + 30))); do
    if port_free "$port"; then
      echo "$port"
      return 0
    fi
  done
  return 1
}

HTTP_PORT="$(pick_port 8098)" || fail "no free HTTP port found near 8098"
AGENT_PORT="$(pick_port 19443)" || fail "no free agent gRPC port found near 19443"
BASE_URL="http://localhost:${HTTP_PORT}"
log "using HTTP port $HTTP_PORT, agent port $AGENT_PORT"

# --- build ----------------------------------------------------------------

log "building web frontend (web/dist)"
(cd "$REPO_ROOT/web" && npm run build >/dev/null) || fail "web build failed"

log "building control plane and CLI binaries"
(cd "$REPO_ROOT" && go build -tags embedweb -o "$BIN" ./cmd/levelrail) || fail "control plane build failed"
(cd "$REPO_ROOT" && go build -o "$CLI" ./cmd/levelrail-cli) || fail "CLI build failed"

# --- start the control plane -----------------------------------------------
#
# -tags embedweb (needed for the real frontend) compiles out the
# APP_DEV_MODE bypass entirely (internal/api/devmode_release.go), so
# dev-fixtures.yml's fixed tokens are not available here. APP_ADMIN_USERNAME/
# PASSWORD bootstrap a real dev/dev admin account instead, unconditionally
# available regardless of build tags.
#
# PLAYWRIGHT_BROWSERS_PATH is pinned to the real HOME's already-installed
# browser cache before HOME is overridden: Playwright resolves that cache
# relative to HOME by default, and the whole point of the override below
# is to keep the control plane's and CLI's own config/state confined to
# the scratch dir instead of touching the real one.
export PLAYWRIGHT_BROWSERS_PATH="${PLAYWRIGHT_BROWSERS_PATH:-$HOME/Library/Caches/ms-playwright}"
export HOME="$FAKE_HOME"
APP_ADMIN_USERNAME=dev \
  APP_ADMIN_PASSWORD=dev \
  APP_DATA_DIR="$DATA_DIR" \
  APP_HTTP_ADDR=":${HTTP_PORT}" \
  APP_AGENT_ADDR=":${AGENT_PORT}" \
  APP_MESH_ENABLED=1 \
  "$BIN" >"$SERVER_LOG" 2>&1 &
SERVER_PID=$!
log "control plane starting (pid $SERVER_PID), log at $SERVER_LOG"

i=0
until curl -s -o /dev/null "$BASE_URL/api/v1/brand"; do
  i=$((i + 1))
  if [ "$i" -ge 30 ]; then
    tail -40 "$SERVER_LOG"
    fail "control plane did not become healthy within 30s"
  fi
  kill -0 "$SERVER_PID" 2>/dev/null || { tail -40 "$SERVER_LOG"; fail "control plane exited early"; }
  sleep 1
done
log "control plane healthy"

# Renaming the single-node mesh bootstrap row: it defaults to
# os.Hostname() (cmd/levelrail/mesh.go's bootstrapLocalNode), which would
# otherwise put the maintainer's real machine hostname into a checked-in
# public screenshot.
i=0
until [ "$(sqlite3 "$DATA_DIR/levelrail.db" 'SELECT COUNT(*) FROM nodes;' 2>/dev/null || echo 0)" != "0" ]; do
  i=$((i + 1))
  [ "$i" -ge 15 ] && break
  sleep 1
done
sqlite3 "$DATA_DIR/levelrail.db" "UPDATE nodes SET name = 'primary';" 2>/dev/null || true

# --- browser login, reused for both shot-scraper and device-login approval ---

log "logging in via the real /login form"
LEVELRAIL_URL="$BASE_URL" python3 "$SCRIPT_DIR/login_state.py" "$STORAGE_STATE" "$SESSION_COOKIE_FILE" \
  || fail "browser login failed"
SESSION_COOKIE="$(cat "$SESSION_COOKIE_FILE")"

# --- mint a CLI API token via the device-login flow ------------------------
#
# The same embedweb-disables-dev-mode constraint above means the CLI has
# no dev-fixtures.yml token to use either. "auth login" (username/
# password) needs https for its session-cookie round trip; --device is
# the one auth path documented to work over plain http, and approving it
# with the session cookie above is a real approval through the real API,
# not a mock.
log "starting CLI device login"
"$CLI" auth login --device --api-url "$BASE_URL" --client-name screenshot-pipeline \
  >"$DEVICE_LOGIN_LOG" 2>&1 &
DEVICE_LOGIN_PID=$!

USER_CODE=""
i=0
while [ -z "$USER_CODE" ]; do
  USER_CODE="$(grep -oE 'enter this code: [A-Za-z0-9-]+' "$DEVICE_LOGIN_LOG" 2>/dev/null | awk '{print $NF}')"
  i=$((i + 1))
  if [ "$i" -ge 15 ]; then
    cat "$DEVICE_LOGIN_LOG"
    fail "CLI device login never printed a code"
  fi
  [ -z "$USER_CODE" ] && sleep 1
done
log "approving device login code $USER_CODE"
approve_status="$(curl -s -o /dev/null -w '%{http_code}' -X POST \
  -H "Cookie: session_token=${SESSION_COOKIE}" \
  "$BASE_URL/api/v1/auth/device/${USER_CODE}/approve")"
[ "$approve_status" = "204" ] || fail "device login approval returned $approve_status, expected 204"

wait "$DEVICE_LOGIN_PID"
DEVICE_LOGIN_PID=""
grep -q "device login approved" "$DEVICE_LOGIN_LOG" || fail "CLI device login did not complete: $(cat "$DEVICE_LOGIN_LOG")"
log "CLI authenticated"

# --- deploy real apps -------------------------------------------------------

wait_for_running() {
  local app="$1" i=0
  until "$CLI" apps network "$app" 2>/dev/null | grep -q "running:.*true"; do
    i=$((i + 1))
    [ "$i" -ge 30 ] && fail "$app never reached running state"
    sleep 2
  done
}

host_port_for() {
  "$CLI" apps network "$1" 2>/dev/null | awk '/host port:/{print $NF}'
}

send_traffic() {
  local port="$1" i
  for i in $(seq 1 6); do
    curl -s -o /dev/null "http://localhost:${port}/"
    curl -s -o /dev/null "http://localhost:${port}/missing-page-${i}"
  done
}

log "creating $HERO_APP"
"$CLI" apps create --name "$HERO_APP" --image nginx:alpine --port 80 || fail "create $HERO_APP failed"
log "creating $SECOND_APP"
"$CLI" apps create --name "$SECOND_APP" --image httpd:alpine --port 80 || fail "create $SECOND_APP failed"

wait_for_running "$HERO_APP"
wait_for_running "$SECOND_APP"
send_traffic "$(host_port_for "$HERO_APP")"

log "triggering second deploy on $HERO_APP"
"$CLI" apps deploy "$HERO_APP" --image nginx:1.27-alpine || fail "deploy 2 on $HERO_APP failed"
wait_for_running "$HERO_APP"

log "triggering third deploy on $HERO_APP (real rollback-shaped history)"
"$CLI" apps deploy "$HERO_APP" --image nginx:alpine || fail "deploy 3 on $HERO_APP failed"
wait_for_running "$HERO_APP"
send_traffic "$(host_port_for "$HERO_APP")"

# --- let metrics accumulate -------------------------------------------------
#
# 15s collection resolution (see ADR on observability): a handful of
# real points needs roughly a minute, not a bare page-load instant.
log "waiting ~75s for metrics history to accumulate"
sleep 75

# --- capture ----------------------------------------------------------------

sed "s#__BASE_URL__#${BASE_URL}#g" "$SCRIPT_DIR/shots.yml" >"$RENDERED_SHOTS"

log "capturing screenshots"
(cd "$REPO_ROOT" && shot-scraper multi "$RENDERED_SHOTS" --auth "$STORAGE_STATE") \
  || fail "shot-scraper failed"

log "done: docs/assets/screenshots/*.png written"
