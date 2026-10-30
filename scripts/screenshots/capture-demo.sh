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
# Env: OUT_DIR (default docs/assets/screenshots), KEEP_SCRATCH=1.
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

log "building web frontend (web/dist)"
# Demo thresholds keep the maintainer's real disk pressure out of the shots.
(cd "$REPO_ROOT/web" && VITE_DISK_WARN_FREE_PERCENT=0.5 VITE_DISK_CRITICAL_FREE_PERCENT=0.1 npm run build >/dev/null) || fail "web build failed"

log "building control plane binary"
(cd "$REPO_ROOT" && go build -tags embedweb -o "$BIN" ./cmd/levelrail) || fail "control plane build failed"

# --- seed and serve -----------------------------------------------------------

export PLAYWRIGHT_BROWSERS_PATH="${PLAYWRIGHT_BROWSERS_PATH:-$HOME/Library/Caches/ms-playwright}"

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

DARK_CFG="$SCRATCH_DIR/shots.dark.yml"
LIGHT_CFG="$SCRATCH_DIR/shots.light.yml"
sed -e "s#__BASE_URL__#${BASE_URL}#g" -e "s#__OUT_DIR__#${OUT_DIR}#g" -e "s#__CHROME_JS__#${CHROME_JS_SED}#g" \
  "$SCRIPT_DIR/shots-demo.yml" >"$DARK_CFG"
sed -E '/^- output:/ s#\.png$#-light.png#' "$DARK_CFG" >"$LIGHT_CFG"

log "capturing dark screenshots"
(cd "$REPO_ROOT" && shot-scraper multi "$DARK_CFG" --auth "$STATE_DARK" --timeout 90000) || fail "dark shot-scraper run failed"
log "capturing light screenshots"
(cd "$REPO_ROOT" && shot-scraper multi "$LIGHT_CFG" --auth "$STATE_LIGHT" --timeout 90000) || fail "light shot-scraper run failed"

# --- optimize ---------------------------------------------------------------------------
python3 - "$OUT_DIR" "$CAPTURE_START" <<'PY' || log "optimizer step skipped (Pillow unavailable)"
import io, os, sys
from PIL import Image

out_dir, started = sys.argv[1], float(sys.argv[2])
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

log "done: screenshots written to $OUT_DIR (dark and -light)"
