#!/usr/bin/env bash
#
# capture.sh: end-to-end pipeline for docs/assets/screenshots/*.png (see
# docs/screenshots.md). Builds fresh binaries, runs a real control plane
# against a scratch data dir, deploys two real app containers plus a real
# managed database, logs in through the actual /login form, and drives
# shot-scraper against the real running dashboard. Safely re-runnable:
# everything lives under a throwaway scratch dir and gets torn down on
# exit, including on error (the EXIT trap runs regardless).
#
# Requires: Go, Node/npm (for web/dist), Docker, Python 3 with
# playwright installed, and shot-scraper on PATH (pip install
# shot-scraper && shot-scraper install).
set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"

HERO_APP="marketing-site"
SECOND_APP="edge-cache"
SAMPLE_DB="docs-sample-db"

SCRATCH_DIR="$(mktemp -d "${TMPDIR:-/tmp}/levelrail-screenshot.XXXXXX")"
BIN="$SCRATCH_DIR/levelrail"
CLI="$SCRATCH_DIR/levelrail-cli"
DATA_DIR="$SCRATCH_DIR/data"
FAKE_HOME="$SCRATCH_DIR/home"
SERVER_LOG="$SCRATCH_DIR/server.log"
DEVICE_LOGIN_LOG="$SCRATCH_DIR/device-login.log"
STORAGE_STATE="$SCRATCH_DIR/storage_state.json"
SESSION_COOKIE_FILE="$SCRATCH_DIR/session_cookie.txt"
CAPTURE_START="$(date +%s)"
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
      "$CLI" databases delete "$SAMPLE_DB" >/dev/null 2>&1
      i=0
      while docker ps --format '{{.Names}}' 2>/dev/null | grep -qE "^(${HERO_APP}|${SECOND_APP}|db-${SAMPLE_DB})"; do
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
  # Fallback: apps/databases delete only tears a container down via the
  # next reconcile pass, which needs the server alive; anything the loop
  # above timed out on gets removed directly so a failed run never
  # leaks containers.
  for name in "$HERO_APP" "$SECOND_APP"; do
    docker ps -a --format '{{.Names}}' 2>/dev/null | grep -E "^${name}-" | xargs -r docker rm -f >/dev/null 2>&1
  done
  docker rm -f "db-${SAMPLE_DB}" >/dev/null 2>&1
  # docker.Runtime has no RemoveVolume (see dataVolumeName's own doc
  # comment in internal/reconcile/database), so the data volume survives
  # "databases delete" and needs its own cleanup here.
  docker volume rm -f "db-${SAMPLE_DB}-data" "db-${SAMPLE_DB}-certs" >/dev/null 2>&1
  # Per-app networks are named after the app and outlive the containers, and a
  # leftover one keeps its old subnet (and so its IPs) on the next run.
  for name in "$HERO_APP" "$SECOND_APP"; do
    docker network rm "Levelrail-app-${name}" >/dev/null 2>&1
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
# The disk-pressure banner reads the host's real free space. Demo thresholds
# are set at build time so a nearly full maintainer disk does not show up in
# every screenshot (VITE_DISK_* in web/src/lib/diskPressure.ts).
(cd "$REPO_ROOT/web" && VITE_DISK_WARN_FREE_PERCENT=0.5 VITE_DISK_CRITICAL_FREE_PERCENT=0.1 npm run build >/dev/null) || fail "web build failed"

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
# load-balancer.png needs a real, populated load balancer view. The load
# balancer feature is gated behind APP_EXPERIMENTAL (internal/experimental):
# this both wires up the ingress reconciler's live upstream registry on the
# server (main.go's experimental.Enabled(LoadBalancer) checks) and unlocks
# the CLI's "lb" subcommand used in the scaling step below.
export APP_EXPERIMENTAL=load-balancer,plugins
# APP_AUTO_PLACEMENT=false: with mesh on, bootstrapLocalNode gives this
# solo control plane one real node row, so auto-placement would assign
# new apps that node's ID instead of leaving node_id empty. Nothing ever
# dials in as that node's agent (single-node has none), so the reconciler
# can never resolve a transport for it. Keeping node_id empty resolves to
# this process's own local runtime directly instead.
# Demo addresses from the RFC 5737 / RFC 2544 documentation ranges, so no
# real public IP, hostname, or LAN address ends up in a screenshot.
APP_ADMIN_USERNAME=dev \
  APP_ADMIN_PASSWORD=dev \
  APP_DATA_DIR="$DATA_DIR" \
  APP_HTTP_ADDR=":${HTTP_PORT}" \
  APP_AGENT_ADDR=":${AGENT_PORT}" \
  APP_MESH_ENABLED=1 \
  APP_AUTO_PLACEMENT=false \
  APP_PUBLIC_HOST=203.0.113.5 \
  APP_BRAND_SUPPORT_EMAIL=support@example.com \
  APP_DASHBOARD_URL="$BASE_URL" \
  APP_AUTH_ENGINE_WEBAUTHN_RP_ID=localhost \
  APP_AUTH_ENGINE_WEBAUTHN_ORIGINS="$BASE_URL" \
  HTTPS_PROXY=http://127.0.0.1:9 HTTP_PROXY=http://127.0.0.1:9 NO_PROXY=127.0.0.1,localhost \
  APP_AGENT_ADVERTISE_HOST=203.0.113.5 \
  APP_MESH_CIDR=198.18.0.0/16 \
  APP_NETWORK_POOL=198.18.0.0/14 \
  APP_DOCTOR_PUBLIC_IP_ENDPOINT=http://127.0.0.1:9/ \
  APP_PUBLIC_IP_DETECT=off \
  APP_API_RATE_LIMIT_READ_RPM=100000 \
  APP_API_RATE_LIMIT_WRITE_RPM=100000 \
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
  USER_CODE="$(grep -oE '(enter this code|confirm this code matches): [A-Za-z0-9-]+' "$DEVICE_LOGIN_LOG" 2>/dev/null | awk '{print $NF}')"
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

# Database containers use a fixed "db-<name>" name (no hash suffix, see
# containerName in internal/reconcile/database/controller.go), so this
# just checks for that exact name rather than a prefix match.
wait_for_db_running() {
  local db="$1" i=0
  until docker ps --format '{{.Names}}' 2>/dev/null | grep -qx "db-${db}"; do
    i=$((i + 1))
    [ "$i" -ge 30 ] && fail "$db database never reached running state"
    sleep 2
  done
}

log "creating $HERO_APP"
"$CLI" apps create --name "$HERO_APP" --image nginx:alpine --port 80 || fail "create $HERO_APP failed"
log "creating $SECOND_APP"
"$CLI" apps create --name "$SECOND_APP" --image httpd:alpine --port 80 || fail "create $SECOND_APP failed"
log "creating $SAMPLE_DB (for managing-databases.md's screenshots)"
"$CLI" databases create --name "$SAMPLE_DB" --engine postgres --version 16 || fail "create $SAMPLE_DB failed"

wait_for_running "$HERO_APP"
wait_for_running "$SECOND_APP"
wait_for_db_running "$SAMPLE_DB"
send_traffic "$(host_port_for "$HERO_APP")"

log "triggering second deploy on $HERO_APP"
"$CLI" apps deploy "$HERO_APP" --image nginx:1.27-alpine || fail "deploy 2 on $HERO_APP failed"
wait_for_running "$HERO_APP"

log "triggering third deploy on $HERO_APP (real rollback-shaped history)"
"$CLI" apps deploy "$HERO_APP" --image nginx:alpine || fail "deploy 3 on $HERO_APP failed"
wait_for_running "$HERO_APP"
send_traffic "$(host_port_for "$HERO_APP")"

# --- scale up and configure a load balancer ---------------------------------
#
# load-balancer.png needs real upstreams, not an empty "not configured"
# state. "apps create"/"apps deploy" have no --replicas flag, so this goes
# straight at PUT /api/v1/apps/{name} (replicas is a plain field on that
# resource) using the CLI's own device-login token. Container names for
# replicas follow application.ReplicaContainerName: "<app>-<8hex>" for
# replica 0, "<app>-<8hex>-r<N>" for replica N>0, both still prefixed with
# "<app>-", so the existing cleanup trap's prefix match already tears them
# down along with everything else.
wait_for_replica_count() {
  local app="$1" want="$2" i=0
  until [ "$(docker ps --format '{{.Names}}' 2>/dev/null | grep -cE "^${app}-")" -ge "$want" ]; do
    i=$((i + 1))
    [ "$i" -ge 30 ] && fail "$app never reached $want running replicas"
    sleep 2
  done
}

CRED_FILE="$FAKE_HOME/.config/levelrail-cli/credentials"
API_TOKEN="$(awk -F= '/^APP_API_TOKEN=/{print $2}' "$CRED_FILE" 2>/dev/null | tr -d '[:space:]')"
[ -n "$API_TOKEN" ] || fail "could not read CLI API token from $CRED_FILE"

log "setting an example secret on $HERO_APP for the environment tab screenshot"
secret_status="$(curl -s -o /dev/null -w '%{http_code}' -X PUT \
  -H "Authorization: Bearer ${API_TOKEN}" -H "Content-Type: application/json" \
  -d '{"value":"docs-example-secret-value"}' \
  "$BASE_URL/api/v1/apps/${HERO_APP}/secrets/API_KEY")"
[ "$secret_status" = "204" ] || fail "setting example secret on $HERO_APP returned $secret_status, expected 204"

log "scaling $HERO_APP to 2 replicas for the load balancer screenshot"
# domains is set here too: the ingress reconciler only ever calls
# planLoadBalancer for a service with at least one active host (an
# operator domain or the zero-config sslip.io fallback), so without one
# the load balancer stays permanently "never observed" no matter how
# many replicas are running. example.com is IANA-reserved for exactly
# this: it never issues a real cert and never resolves to this machine,
# so ACME just fails quietly in the background.
scale_status="$(curl -s -o /dev/null -w '%{http_code}' -X PUT \
  -H "Authorization: Bearer ${API_TOKEN}" -H "Content-Type: application/json" \
  -d '{"image":"nginx:alpine","port":80,"replicas":2,"domains":["marketing-site.example.com"]}' \
  "$BASE_URL/api/v1/apps/${HERO_APP}")"
[ "$scale_status" = "200" ] || fail "scale $HERO_APP to 2 replicas returned $scale_status, expected 200"
wait_for_replica_count "$HERO_APP" 2

log "configuring load balancer on $HERO_APP"
"$CLI" lb set "$HERO_APP" --algorithm round_robin --health-path / --health-interval 5s --health-timeout 2s \
  || fail "lb set on $HERO_APP failed"

log "waiting for the ingress reconciler to observe both upstreams"
i=0
until [ "$("$CLI" lb status "$HERO_APP" --json 2>/dev/null | jq '[.upstreams[] | select(.dial != "")] | length' 2>/dev/null)" -ge 2 ] 2>/dev/null; do
  i=$((i + 1))
  [ "$i" -ge 30 ] && fail "$HERO_APP load balancer never reported 2 upstreams"
  sleep 2
done
"$CLI" lb check "$HERO_APP" || fail "lb check on $HERO_APP failed"

# --- seed pages that need demo data -----------------------------------------
#
# Demo content for the pages shots.yml lists beyond apps and databases:
# a pipeline with one run, an export destination, and an archive. Each one
# is best-effort except the ids the deploy-log and pipeline-run shots name.
api() { curl -s -H "Authorization: Bearer ${API_TOKEN}" -H "Content-Type: application/json" "$@"; }

log "seeding a pipeline, a run, an export destination and an archive"
pipeline_body="$(jq -n --arg yaml "$(cat <<'YAML'
version: 1
name: release
on:
  manual: {}
  api: true
stages: [test, deploy]
jobs:
  test:
    stage: test
    image: alpine:3.20
    steps:
      - run: echo "unit tests passed"
  ship:
    stage: deploy
    image: alpine:3.20
    steps:
      - run: echo "release shipped"
YAML
)" '{name: "release", yaml: $yaml, enabled: true}')"
api -X POST -d "$pipeline_body" "$BASE_URL/api/v1/apps/${HERO_APP}/pipelines" >"$SCRATCH_DIR/seed-pipeline.json" \
  || log "pipeline seed failed"
run_id="$(api -X POST -d '{}' "$BASE_URL/api/v1/apps/${HERO_APP}/pipelines/release/runs" | tee -a "$SCRATCH_DIR/seed-pipeline.json" | jq -r '.id // empty')"
[ -n "$run_id" ] || fail "pipeline run for $HERO_APP did not start: $(cat "$SCRATCH_DIR/seed-pipeline.json")"
sleep 3

api -o /dev/null -X POST -d '{"name":"ops-log-sink","kind":"logs_http","endpoint":"https://logs.example.com/ingest","apps":["'"$HERO_APP"'"],"min_level":"info"}' \
  "$BASE_URL/api/v1/export/destinations" || log "export destination seed failed"
api -o /dev/null -X POST -d '{"kind":"deploys","older_than_days":30}' "$BASE_URL/api/v1/archives" || log "archive seed failed"
# Without a domain the auto sslip.io URL carries this machine's public IP, which would end up in the apps list shot.
api -o /dev/null -X PUT -d '{"image":"httpd:alpine","port":80,"domains":["edge-cache.example.com"]}' \
  "$BASE_URL/api/v1/apps/${SECOND_APP}" || log "edge-cache domain seed failed"

deploy_id="$(api "$BASE_URL/api/v1/apps/${HERO_APP}/deploy-attempts" \
  | jq -r '(if type=="array" then . else (.attempts // .items // []) end) | .[0].id // empty')"
[ -n "$deploy_id" ] || fail "no deploy attempt found for $HERO_APP"

# --- let metrics accumulate -------------------------------------------------
#
# 15s collection resolution (see ADR on observability): a handful of
# real points needs roughly a minute, not a bare page-load instant.
log "waiting ~75s for metrics history to accumulate"
sleep 75

# --- capture ----------------------------------------------------------------
#
# Dark shots keep the shots.yml output names; light shots add a -light
# suffix. The theme is pinned through the levelrail-theme localStorage key
# (ThemeProvider.tsx) by writing it into a copy of the login storage state.
STATE_DARK="$SCRATCH_DIR/storage_state_dark.json"
STATE_LIGHT="$SCRATCH_DIR/storage_state_light.json"
python3 - "$STORAGE_STATE" "$STATE_DARK" "$STATE_LIGHT" "$BASE_URL" <<'PY' || fail "could not write theme storage states"
import json, sys
src, dark, light, origin = sys.argv[1:5]
base = json.load(open(src))
for path, theme in ((dark, "dark"), (light, "light")):
    state = dict(base)
    origins = []
    for o in base.get("origins", []):
        if o.get("origin") != origin:
            origins.append(o)
            continue
        # Keep the login's localStorage (the username the route guard reads), only set the theme.
        items = [i for i in o.get("localStorage", []) if i.get("name") != "levelrail-theme"]
        items.append({"name": "levelrail-theme", "value": theme})
        origins.append({**o, "localStorage": items})
    if not any(o.get("origin") == origin for o in origins):
        origins.append({"origin": origin, "localStorage": [{"name": "levelrail-theme", "value": theme}]})
    state["origins"] = origins
    json.dump(state, open(path, "w"))
PY

DARK_CFG="$SCRATCH_DIR/shots.dark.yml"
LIGHT_CFG="$SCRATCH_DIR/shots.light.yml"
sed -e "s#__BASE_URL__#${BASE_URL}#g" -e "s#__DEPLOY_ID__#${deploy_id}#g" -e "s#__RUN_ID__#${run_id}#g" \
  "$SCRIPT_DIR/shots.yml" >"$DARK_CFG"
sed -E '/^- output:/ s#\.png$#-light.png#' "$DARK_CFG" >"$LIGHT_CFG"

log "checking every shot page for real identifiers (IPs, emails, sslip hosts, hostname)"
python3 "$SCRIPT_DIR/leak_check.py" "$DARK_CFG" "$STATE_DARK" || fail "a shot page shows a real identifier; fix the demo data first"

log "capturing dark screenshots"
# Pages that never reach the load event are captured by slow_shots.py instead.
DARK_FAST="$SCRATCH_DIR/shots.dark.fast.yml"
LIGHT_FAST="$SCRATCH_DIR/shots.light.fast.yml"
SLOW_LOG="$SCRATCH_DIR/slow-shots.log"
python3 "$SCRIPT_DIR/slow_shots.py" split "$DARK_CFG" "$DARK_FAST" || fail "could not split the shot config"
python3 "$SCRIPT_DIR/slow_shots.py" split "$LIGHT_CFG" "$LIGHT_FAST" || fail "could not split the shot config"

(cd "$REPO_ROOT" && shot-scraper multi "$DARK_FAST" --auth "$STATE_DARK" --timeout 90000) || fail "dark shot-scraper run failed"
log "capturing light screenshots"
(cd "$REPO_ROOT" && shot-scraper multi "$LIGHT_FAST" --auth "$STATE_LIGHT" --timeout 90000) || fail "light shot-scraper run failed"

log "capturing slow pages (domcontentloaded, then networkidle); log at $SLOW_LOG"
(cd "$REPO_ROOT" && python3 "$SCRIPT_DIR/slow_shots.py" run "$DARK_CFG" "$STATE_DARK" "$SLOW_LOG") || fail "dark slow-page capture failed"
(cd "$REPO_ROOT" && python3 "$SCRIPT_DIR/slow_shots.py" run "$LIGHT_CFG" "$STATE_LIGHT" "$SLOW_LOG") || fail "light slow-page capture failed"

# --- optimize ------------------------------------------------------------------
#
# Output is re-encoded losslessly and quantized to 256 colors only when it
# is still over 400 KB. Every captured file is rewritten.
python3 - "$REPO_ROOT" "$CAPTURE_START" <<'PY' || log "optimizer step skipped (Pillow unavailable)"
import io, os, sys
from PIL import Image

repo, started = sys.argv[1], float(sys.argv[2])
out_dir = os.path.join(repo, "docs/assets/screenshots")
limit = 400 * 1024

for name in sorted(os.listdir(out_dir)):
    path = os.path.join(out_dir, name)
    if not name.endswith(".png") or os.path.getmtime(path) < started:
        continue
    img = Image.open(path).convert("RGB")
    buf = io.BytesIO()
    img.save(buf, format="PNG", optimize=True)
    if buf.tell() > limit:
        img = img.quantize(colors=256, method=Image.Quantize.FASTOCTREE).convert("RGB")
        buf = io.BytesIO()
        img.save(buf, format="PNG", optimize=True)
    open(path, "wb").write(buf.getvalue())
    print(f"wrote {name} ({len(buf.getvalue()) // 1024} KB)")
PY

log "done: docs/assets/screenshots/*.png written (dark and -light)"
