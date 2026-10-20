#!/usr/bin/env bash
#
# perf-measure.sh: dev-only. Seeds 1000 apps, 200 databases, 50 nodes and
# 20k deploy rows into a throwaway data dir, runs a local control plane on
# it with Docker unreachable, and measures the dashboard in headless
# Chromium (scripts/perf-measure.py). Never touches a remote server;
# everything is deleted on exit. Needs Go, npm, Python 3 with playwright.
set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
SCRATCH="$(mktemp -d "${TMPDIR:-/tmp}/levelrail-perf.XXXXXX")"
PORT="${PERF_PORT:-8197}"
BASE_URL="http://localhost:${PORT}"
SERVER_PID=""
export CGO_ENABLED=0

cleanup() {
  [ -n "$SERVER_PID" ] && kill "$SERVER_PID" 2>/dev/null && wait "$SERVER_PID" 2>/dev/null
  rm -rf "$SCRATCH"
}
trap cleanup EXIT

fail() { echo "[perf-measure] $*" >&2; exit 1; }
export PLAYWRIGHT_BROWSERS_PATH="${PLAYWRIGHT_BROWSERS_PATH:-$HOME/Library/Caches/ms-playwright}"

(cd "$REPO_ROOT/web" && npm run build >/dev/null) || fail "web build failed"
(cd "$REPO_ROOT" && go build -tags embedweb -o "$SCRATCH/levelrail" ./cmd/levelrail) || fail "control plane build failed"
(cd "$REPO_ROOT" && go run ./scripts/perf-fixtures -data-dir "$SCRATCH/data") || fail "seeding failed"

DOCKER_HOST=unix:///nonexistent.sock \
  APP_ADMIN_USERNAME=dev APP_ADMIN_PASSWORD=dev \
  APP_DATA_DIR="$SCRATCH/data" APP_HTTP_ADDR=":${PORT}" APP_AGENT_ADDR=":$((PORT + 1000))" \
  APP_AUTO_PLACEMENT=false APP_PUBLIC_IP_DETECT=off \
  APP_DASHBOARD_URL="$BASE_URL" \
  APP_API_RATE_LIMIT_READ_RPM=1000000 APP_API_RATE_LIMIT_WRITE_RPM=1000000 \
  "$SCRATCH/levelrail" >"$SCRATCH/server.log" 2>&1 &
SERVER_PID=$!

i=0
until curl -s -o /dev/null "$BASE_URL/api/v1/brand"; do
  i=$((i + 1)); [ "$i" -ge 60 ] && fail "control plane did not start: $(tail -5 "$SCRATCH/server.log")"
  sleep 1
done

LEVELRAIL_URL="$BASE_URL" python3 "$SCRIPT_DIR/screenshots/login_state.py" "$SCRATCH/state.json" >/dev/null || fail "login failed"
python3 "$SCRIPT_DIR/perf-measure.py" "$BASE_URL" "$SCRATCH/state.json" "${PERF_RUNS:-3}"
