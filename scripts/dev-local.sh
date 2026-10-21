#!/usr/bin/env bash
# Runs a local control plane (dev mode, login dev/dev) plus the Vite dev server
# from this worktree so new features can be seen as they merge.
#
# Usage: scripts/dev-local.sh start|stop|restart|status|watch
# watch = start, then rebuild and restart only the API when a .go file changes
# (the web side already hot-reloads through Vite). A failed build keeps the old
# server running. LOCAL_WATCH_INTERVAL sets the poll seconds (default 2).
# UI http://127.0.0.1:5173, API http://127.0.0.1:8080, login dev/dev.
# Data and logs live in ${LOCAL_DATA_DIR:-$HOME/levelrail-local-data}.

set -uo pipefail

repo_root="$(git rev-parse --show-toplevel)"
dir="${LOCAL_DATA_DIR:-$HOME/levelrail-local-data}"
api_port="${LOCAL_API_PORT:-8080}"
web_port="${LOCAL_WEB_PORT:-5173}"
mkdir -p "$dir"

listening() { lsof -iTCP:"$1" -sTCP:LISTEN -t 2>/dev/null; }

stop_port() {
	local pid _
	pid="$(listening "$1")"
	[ -n "$pid" ] || return 0
	kill $pid 2>/dev/null
	# The server drains with no grace limit, so wait for the port and then force it:
	# a new server started while the old one still holds the port never binds.
	for _ in $(seq 1 20); do
		[ -z "$(listening "$1")" ] && return 0
		sleep 0.5
	done
	kill -9 $pid 2>/dev/null
	sleep 0.5
}

stop_api() { stop_port "$api_port"; }

stop() {
	stop_port "$api_port"
	stop_port "$web_port"
}

# build_binary compiles the control plane to $1. A linked worktree stamps the
# primary checkout's commit into the build, so name the real one.
build_binary() {
	local ver; ver="${LOCAL_VERSION_BASE:-v0.8.0}-dev.$(git -C "$repo_root" rev-parse --short HEAD)"
	(cd "$repo_root" && go build -ldflags "-X github.com/GLINCKER/levelrail/internal/version.Version=$ver" -o "$1" ./cmd/levelrail)
}

start_api() {
	# Env-gated experimental features are not on the Beta features page; turn them
	# on locally so work in progress is visible. LOCAL_EXPERIMENTAL overrides.
	(cd "$repo_root" && APP_EXPERIMENTAL="${LOCAL_EXPERIMENTAL:-load-balancer,ai-models,iac,cloudflare-tunnel,access-roles,global-environments,ai-control}" \
		APP_DEV_MODE=1 APP_DEV_FIXTURES_FILE="$repo_root/test/fixtures/dev-fixtures.yml" \
		APP_DATA_DIR="$dir/data" APP_HTTP_ADDR="127.0.0.1:$api_port" APP_AGENT_ADDR="127.0.0.1:0" \
		APP_INGRESS_HTTP_ADDR="127.0.0.1:0" APP_INGRESS_HTTPS_ADDR="127.0.0.1:0" \
		nohup "$dir/levelrail" >"$dir/server.log" 2>&1 &)
}

wait_api() {
	local _
	for _ in $(seq 1 60); do
		curl -fsS -o /dev/null "http://127.0.0.1:$api_port/api/v1/brand" 2>/dev/null && return 0
		sleep 0.5
	done
	return 1
}

start() {
	[ -z "$(listening "$api_port")" ] || { echo "dev-local: port $api_port already in use, run restart" >&2; return 1; }
	build_binary "$dir/levelrail" || return 1
	start_api
	if [ -z "$(listening "$web_port")" ]; then
		(cd "$repo_root/web" && LOCAL_API_PORT="$api_port" nohup npx vite --host 127.0.0.1 --port "$web_port" >"$dir/vite.log" 2>&1 &)
	fi
	if wait_api; then
		echo "dev-local: up. UI http://127.0.0.1:$web_port  API http://127.0.0.1:$api_port  login dev/dev"
		return 0
	fi
	echo "dev-local: API did not come up, see $dir/server.log" >&2
	tail -10 "$dir/server.log" >&2
	return 1
}

status() {
	echo "api  :$api_port $([ -n "$(listening "$api_port")" ] && echo up || echo down)"
	echo "web  :$web_port $([ -n "$(listening "$web_port")" ] && echo up || echo down)"
}

watch() {
	local interval="${LOCAL_WATCH_INTERVAL:-2}" stamp="$dir/.watch-stamp" changed
	stop
	start || return 1
	touch "$stamp"
	trap 'stop; exit 0' INT TERM
	echo "dev-local: watching Go sources every ${interval}s (web hot-reloads through Vite), Ctrl-C stops both"
	while sleep "$interval"; do
		changed="$(cd "$repo_root" && find cmd internal kit proto -name '*.go' -newer "$stamp" 2>/dev/null | head -1)"
		[ -n "$changed" ] || continue
		# Touch before building so an edit made mid-build triggers another pass.
		touch "$stamp"
		echo "dev-local: $changed changed, rebuilding"
		if build_binary "$dir/levelrail.next"; then
			stop_api
			mv "$dir/levelrail.next" "$dir/levelrail"
			start_api
			wait_api && echo "dev-local: API restarted" || echo "dev-local: API did not come up, see $dir/server.log" >&2
		else
			echo "dev-local: build failed, keeping the running server" >&2
		fi
	done
}

case "${1:-}" in
start) start ;;
stop) stop ;;
restart) stop; start ;;
status) status ;;
watch) watch ;;
*) echo "usage: $0 start|stop|restart|status|watch" >&2; exit 2 ;;
esac
