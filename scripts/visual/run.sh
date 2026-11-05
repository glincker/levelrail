#!/usr/bin/env bash
# Builds and starts a throwaway control plane (dev mode, temp data dir, free
# ports) plus the Vite dev server, runs scripts/visual/run.py, then tears down.
# Usage: scripts/visual/run.sh [run.py args]   (env: VISUAL_OUT, VISUAL_BIN)
set -uo pipefail

repo="$(cd "$(dirname "$0")/../.." && pwd)"
out="${VISUAL_OUT:-$repo/web/visual-out}"
work="$(mktemp -d "${TMPDIR:-/tmp}/lr-visual.XXXXXX")"
bin="${VISUAL_BIN:-$work/levelrail}"

free_port() { python3 -c 'import socket;s=socket.socket();s.bind(("127.0.0.1",0));print(s.getsockname()[1])'; }
api_port="$(free_port)"
web_port="$(free_port)"
pids=()

cleanup() {
	local p
	for p in "${pids[@]:-}"; do [ -n "$p" ] && kill "$p" 2>/dev/null; done
	sleep 1
	for p in "${pids[@]:-}"; do [ -n "$p" ] && kill -9 "$p" 2>/dev/null; done
	rm -rf "$work"
}
trap cleanup EXIT

if [ ! -x "$bin" ]; then
	echo "visual: building control plane" >&2
	(cd "$repo" && go build -buildvcs=false -o "$bin" ./cmd/levelrail) || { echo "visual: build failed" >&2; exit 2; }
fi

mkdir -p "$work/data"
cd "$repo" || exit 2
APP_DEV_MODE=1 APP_MAIL_DEMO=1 APP_DEV_FIXTURES_FILE="$repo/test/fixtures/dev-fixtures.yml" \
	APP_EXPERIMENTAL="load-balancer,ai-models,iac,cloudflare-tunnel,access-roles,global-environments,ai-control,mail" \
	APP_DATA_DIR="$work/data" APP_HTTP_ADDR="127.0.0.1:$api_port" APP_AGENT_ADDR="127.0.0.1:0" \
	APP_INGRESS_HTTP_ADDR="127.0.0.1:0" APP_INGRESS_HTTPS_ADDR="127.0.0.1:0" \
	"$bin" >"$work/server.log" 2>&1 &
pids+=("$!")

(cd "$repo/web" && LOCAL_API_PORT="$api_port" exec npx vite --host 127.0.0.1 --port "$web_port" --strictPort >"$work/vite.log" 2>&1) &
pids+=("$!")

wait_url() {
	local _
	for _ in $(seq 1 120); do
		curl -fsS -o /dev/null "$1" 2>/dev/null && return 0
		sleep 0.5
	done
	return 1
}
wait_url "http://127.0.0.1:$api_port/api/v1/brand" || { echo "visual: API did not start" >&2; tail -10 "$work/server.log" >&2; exit 2; }
wait_url "http://127.0.0.1:$web_port/" || { echo "visual: Vite did not start" >&2; tail -10 "$work/vite.log" >&2; exit 2; }

LEVELRAIL_VISUAL_API="http://127.0.0.1:$api_port" LEVELRAIL_VISUAL_URL="http://127.0.0.1:$web_port" \
	python3 "$repo/scripts/visual/run.py" --out "$out" "$@"
exit $?
