#!/usr/bin/env bash
#
# capture-demo.sh: populated hero screenshots from demo mode (see
# docs/screenshots.md). Seeds 14 days of fictional history into a scratch data
# dir, serves it with `levelrail demo serve` (loopback, no Docker), logs in as
# the read-only demo account and shoots the pages in shots-demo.yml, dark and
# -light. The demo banner, tour and blocked-write toast are hidden per shot.
# Everything lives under a throwaway scratch dir removed on exit.
#
# Requires: Go, sqlite3 (optional, release rows), Node/npm, Python 3 with playwright, and shot-scraper on PATH.
# Env: OUT_DIR (default docs/assets/screenshots), SCALE_FACTOR (device pixel ratio, default 1), NO_OPTIMIZE=1, KEEP_SCRATCH=1, SHOT_WORKERS (parallel
# shot-scraper processes per theme, default 4), ONLY="name1,name2" (output names),
# THEMES="dark,light", FORCE_BUILD=1 (ignore the build cache), WAIT_FLOOR_MS (default 800).
set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
OUT_DIR="${OUT_DIR:-$REPO_ROOT/docs/assets/screenshots}"

SCRATCH_DIR="$(mktemp -d "${TMPDIR:-/tmp}/levelrail-demo-screenshot.XXXXXX")"
BIN="$SCRATCH_DIR/levelrail"
DATA_DIR="$SCRATCH_DIR/data"
SERVER_LOG="$SCRATCH_DIR/server.log"
STORAGE_STATE="$SCRATCH_DIR/storage_state.json"
CAPTURE_START="$(date +%s)"
T_START="$CAPTURE_START"
SHOT_WORKERS="${SHOT_WORKERS:-4}"
THEMES="${THEMES:-dark,light}"
ONLY="${ONLY:-}"
WAIT_FLOOR_MS="${WAIT_FLOOR_MS:-800}"
CACHE_DIR="${XDG_CACHE_HOME:-$HOME/.cache}/levelrail-screenshots"
mkdir -p "$CACHE_DIR"
mkdir -p "$DATA_DIR" "$OUT_DIR"

SERVER_PID=""

log() { echo "[capture-demo.sh] $*"; }

cleanup() {
  if [ -n "$SERVER_PID" ] && kill -0 "$SERVER_PID" 2>/dev/null; then
    kill "$SERVER_PID" 2>/dev/null
    wait "$SERVER_PID" 2>/dev/null
  fi
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

HTTP_PORT="$(pick_port 8095)" || fail "no free HTTP port found near 8095"
BASE_URL="http://127.0.0.1:${HTTP_PORT}"
log "using HTTP port $HTTP_PORT"

# --- build ----------------------------------------------------------------

# Binary and web/dist live in the cache dir so reruns reuse them. The stamp
# covers HEAD plus uncommitted and untracked changes under the built dirs.
T_BUILD0="$(date +%s)"
CACHED_BIN="$CACHE_DIR/levelrail"
CACHED_DIST="$CACHE_DIR/web-dist"
STAMP_FILE="$CACHE_DIR/build.stamp"
STAMP="$(
  cd "$REPO_ROOT" && {
    git rev-parse HEAD
    git diff HEAD -- web cmd internal
    git ls-files -o --exclude-standard -- web cmd internal | grep -v '^web/dist/' | while read -r f; do echo "$f"; cat "$f"; done
    echo "disk:0.5/0.1"
  } | shasum | cut -d' ' -f1
)"

if [ "${FORCE_BUILD:-0}" != "1" ] && [ -x "$CACHED_BIN" ] && [ "$(cat "$STAMP_FILE" 2>/dev/null)" = "$STAMP" ]; then
  log "build cache hit ($STAMP), skipping web and go build"
  [ -d "$REPO_ROOT/web/dist" ] || { [ -d "$CACHED_DIST" ] && cp -R "$CACHED_DIST" "$REPO_ROOT/web/dist"; }
else
  log "build cache miss, building web frontend (web/dist)"
  # Demo thresholds keep the maintainer's real disk pressure out of the shots.
  (cd "$REPO_ROOT/web" && VITE_DISK_WARN_FREE_PERCENT=0.5 VITE_DISK_CRITICAL_FREE_PERCENT=0.1 npm run build >/dev/null) || fail "web build failed"

  log "building control plane binary"
  (cd "$REPO_ROOT" && go build -tags embedweb -o "$CACHED_BIN.tmp" ./cmd/levelrail) || fail "control plane build failed"
  mv "$CACHED_BIN.tmp" "$CACHED_BIN"
  rm -rf "$CACHED_DIST"
  cp -R "$REPO_ROOT/web/dist" "$CACHED_DIST" || log "could not cache web/dist"
  echo "$STAMP" >"$STAMP_FILE"
fi
cp "$CACHED_BIN" "$BIN"
T_BUILD="$(( $(date +%s) - T_BUILD0 ))"

# --- seed and serve -----------------------------------------------------------

export PLAYWRIGHT_BROWSERS_PATH="${PLAYWRIGHT_BROWSERS_PATH:-$HOME/Library/Caches/ms-playwright}"

T_SEED0="$(date +%s)"
log "seeding demo data"
APP_BRAND_FILE="$REPO_ROOT/brand.yaml" APP_DATA_DIR="$DATA_DIR" "$BIN" demo seed --seed 1 --scenario "${SCENARIO:-calm}" >"$SCRATCH_DIR/seed.log" 2>&1 \
  || { cat "$SCRATCH_DIR/seed.log"; fail "demo seed failed"; }

# Release rows for the app overview Releases card (best effort).
if command -v sqlite3 >/dev/null 2>&1; then
  sqlite3 "$DATA_DIR/levelrail.db" <"$SCRIPT_DIR/demo-extra-seed.sql" || log "extra seed failed, release shots will be empty"
else
  log "sqlite3 not found, skipping extra release seed"
fi

# demo serve refuses to start when any of the unset variables is present.
env -u DOCKER_HOST -u APP_AGENT_ADDR -u APP_INGRESS_HTTP_ADDR -u APP_INGRESS_HTTPS_ADDR \
  APP_BRAND_FILE="$REPO_ROOT/brand.yaml" \
  APP_DEMO_MODE=1 \
  APP_RELEASE_BINDINGS=shadow \
  APP_ANALYSIS_ENABLED=on \
  APP_STATUS_PAGE_SAMPLE_INTERVAL=2s \
  APP_DATA_DIR="$DATA_DIR" \
  APP_HTTP_ADDR="127.0.0.1:${HTTP_PORT}" \
  "$BIN" demo serve >"$SERVER_LOG" 2>&1 &
SERVER_PID=$!
log "demo server starting (pid $SERVER_PID), log at $SERVER_LOG"

i=0
until curl -s "$BASE_URL/api/v1/demo" | grep -q '"enabled":[[:space:]]*true'; do
  i=$((i + 1))
  if [ "$i" -ge 30 ]; then
    tail -40 "$SERVER_LOG"
    fail "demo server did not become ready within 30s"
  fi
  kill -0 "$SERVER_PID" 2>/dev/null || { tail -40 "$SERVER_LOG"; fail "demo server exited early"; }
  sleep 1
done
log "demo server ready"
T_SEED="$(( $(date +%s) - T_SEED0 ))"

# --- login ----------------------------------------------------------------------

log "logging in as the read-only demo account"
LEVELRAIL_URL="$BASE_URL" LEVELRAIL_USER=demo LEVELRAIL_PASS=demo-readonly \
  python3 "$SCRIPT_DIR/login_state.py" "$STORAGE_STATE" || fail "browser login failed"

# --- storage states: theme + tour dismissed ---------------------------------------
#
# demo.tour.dismissed.v1 is the key web/src/lib/demoTour.ts reads; the theme
# is pinned through levelrail-theme (ThemeProvider.tsx).
STATE_DARK="$SCRATCH_DIR/storage_state_dark.json"
STATE_LIGHT="$SCRATCH_DIR/storage_state_light.json"
python3 - "$STORAGE_STATE" "$STATE_DARK" "$STATE_LIGHT" "$BASE_URL" <<'PY' || fail "could not write storage states"
import json, sys
src, dark, light, origin = sys.argv[1:5]
base = json.load(open(src))
managed = {"levelrail-theme", "demo.tour.dismissed.v1"}
for path, theme in ((dark, "dark"), (light, "light")):
    state = dict(base)
    origins = []
    found = False
    extra = [{"name": "levelrail-theme", "value": theme}, {"name": "demo.tour.dismissed.v1", "value": "1"}]
    for o in base.get("origins", []):
        if o.get("origin") != origin:
            origins.append(o)
            continue
        found = True
        items = [i for i in o.get("localStorage", []) if i.get("name") not in managed]
        origins.append({**o, "localStorage": items + extra})
    if not found:
        origins.append({"origin": origin, "localStorage": extra})
    state["origins"] = origins
    json.dump(state, open(path, "w"))
PY

# --- capture ------------------------------------------------------------------------
#
# The banner is a role=status strip; the blocked-write toast lives in the
# toast portal; the floating help bubble is the small fixed element in the
# bottom-right corner. All are hidden with display:none so React keeps its nodes.
CHROME_JS="(() => { const s = document.createElement('style'); s.textContent = '[data-slot=toast-portal],[data-slot=toast-viewport]{display:none !important}'; document.head.appendChild(s); document.querySelectorAll('[role=status]').forEach(e => { if (/read-only demo/i.test(e.textContent || '')) e.style.display = 'none'; }); document.querySelectorAll('button,a,div').forEach(e => { const r = e.getBoundingClientRect(); if (getComputedStyle(e).position === 'fixed' && r.width < 90 && r.height < 90 && r.right > innerWidth - 100 && r.bottom > innerHeight - 100) e.style.display = 'none'; }); })()"

# Escape the sed replacement metacharacters that appear in the snippet.
CHROME_JS_SED="${CHROME_JS//\\/\\\\}"
CHROME_JS_SED="${CHROME_JS_SED//&/\\&}"
CHROME_JS_SED="${CHROME_JS_SED//#/\\#}"

BASE_CFG="$SCRATCH_DIR/shots.base.yml"
sed -e "s#__BASE_URL__#${BASE_URL}#g" -e "s#__OUT_DIR__#${OUT_DIR}#g" -e "s#__CHROME_JS__#${CHROME_JS_SED}#g" \
  "$SCRIPT_DIR/shots-demo.yml" >"$BASE_CFG"

# Page-ready condition: main mounted and no loading skeleton or busy region left.
WAIT_FOR_JS="document.querySelector('main') !== null && !document.querySelector('[aria-busy=true], [data-slot=skeleton]')"

T_CAP0="$(date +%s)"
CHUNKS="$(python3 "$SCRIPT_DIR/shard.py" "$BASE_CFG" "$SCRATCH_DIR/shots" "$SHOT_WORKERS" "$ONLY" "$WAIT_FLOOR_MS" "$WAIT_FOR_JS" "$THEMES")" \
  || fail "could not build shot chunks (check ONLY and THEMES)"
CAPTURE_START="$(date +%s)"

PIDS=()
NAMES=()
for cfg in $CHUNKS; do
  name="$(basename "$cfg" .yml)"
  theme="$(echo "$name" | cut -d. -f2)"
  state="$STATE_DARK"
  [ "$theme" = "light" ] && state="$STATE_LIGHT"
  (cd "$REPO_ROOT" && shot-scraper multi "$cfg" --auth "$state" --timeout 90000 --scale-factor "${SCALE_FACTOR:-1}") >"$SCRATCH_DIR/$name.log" 2>&1 &
  PIDS+=($!)
  NAMES+=("$name")
done
log "capturing ${#PIDS[@]} chunks in parallel (themes: $THEMES, workers per theme: $SHOT_WORKERS)"

failed=0
for idx in "${!PIDS[@]}"; do
  if ! wait "${PIDS[$idx]}"; then
    failed=1
    log "chunk ${NAMES[$idx]} failed:"
    tail -20 "$SCRATCH_DIR/${NAMES[$idx]}.log"
  fi
done
[ "$failed" = "0" ] || fail "shot-scraper run failed"
T_CAP="$(( $(date +%s) - T_CAP0 ))"

# --- optimize ---------------------------------------------------------------------------
T_OPT0="$(date +%s)"
if [ "${NO_OPTIMIZE:-0}" = "1" ]; then log "optimizer skipped (NO_OPTIMIZE=1)"; else python3 "$SCRIPT_DIR/optimize.py" "$OUT_DIR" "$CAPTURE_START" || log "optimizer step skipped (Pillow unavailable)"; fi
T_OPT="$(( $(date +%s) - T_OPT0 ))"

log "done: screenshots written to $OUT_DIR (dark and -light)"
log "timing (s): build=$T_BUILD seed=$T_SEED capture=$T_CAP optimize=$T_OPT total=$(( $(date +%s) - T_START ))"
