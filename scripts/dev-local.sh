#!/usr/bin/env bash
# Runs a local control plane (dev mode, login dev/dev) plus the Vite dev server
# from this worktree so new features can be seen as they merge.
#
# Usage: scripts/dev-local.sh start|stop|restart|status
# UI http://127.0.0.1:5173, API http://127.0.0.1:8080, login dev/dev.
# Data and logs live in ${LOCAL_DATA_DIR:-$HOME/levelrail-local-data}.

set -uo pipefail

repo_root="$(git rev-parse --show-toplevel)"
dir="${LOCAL_DATA_DIR:-$HOME/levelrail-local-data}"
api_port="${LOCAL_API_PORT:-8080}"
web_port="${LOCAL_WEB_PORT:-5173}"
mkdir -p "$dir"

listening() { lsof -iTCP:"$1" -sTCP:LISTEN -t 2>/dev/null; }

stop() {
	local pid
	for port in "$api_port" "$web_port"; do
		pid="$(listening "$port")"
		[ -z "$pid" ] || kill $pid 2>/dev/null
	done
	sleep 1
}

start() {
	[ -z "$(listening "$api_port")" ] || { echo "dev-local: port $api_port already in use, run restart" >&2; return 1; }
	# A linked worktree stamps the primary checkout's commit into the build; name the real one.
	local ver; ver="${LOCAL_VERSION_BASE:-v0.8.0}-dev.$(git -C "$repo_root" rev-parse --short HEAD)"
	(cd "$repo_root" && go build -ldflags "-X github.com/GLINCKER/levelrail/internal/version.Version=$ver" -o "$dir/levelrail" ./cmd/levelrail) || return 1
	# Env-gated experimental features are not on the Beta features page; turn them
	# on locally so work in progress is visible. LOCAL_EXPERIMENTAL overrides.
	(cd "$repo_root" && APP_EXPERIMENTAL="${LOCAL_EXPERIMENTAL:-load-balancer,ai-models,iac,cloudflare-tunnel,access-roles,global-environments,ai-control}" \
		APP_DEV_MODE=1 APP_DEV_FIXTURES_FILE="$repo_root/test/fixtures/dev-fixtures.yml" \
		APP_DATA_DIR="$dir/data" APP_HTTP_ADDR="127.0.0.1:$api_port" APP_AGENT_ADDR="127.0.0.1:0" \
		APP_INGRESS_HTTP_ADDR="127.0.0.1:0" APP_INGRESS_HTTPS_ADDR="127.0.0.1:0" \
		nohup "$dir/levelrail" >"$dir/server.log" 2>&1 &)
	if [ -z "$(listening "$web_port")" ]; then
		(cd "$repo_root/web" && LOCAL_API_PORT="$api_port" nohup npx vite --host 127.0.0.1 --port "$web_port" >"$dir/vite.log" 2>&1 &)
	fi
	local i
	for i in $(seq 1 60); do
		if curl -fsS -o /dev/null "http://127.0.0.1:$api_port/api/v1/brand" 2>/dev/null; then
			echo "dev-local: up. UI http://127.0.0.1:$web_port  API http://127.0.0.1:$api_port  login dev/dev"
			return 0
		fi
		sleep 0.5
	done
	echo "dev-local: API did not come up, see $dir/server.log" >&2
	tail -10 "$dir/server.log" >&2
	return 1
}

status() {
	echo "api  :$api_port $([ -n "$(listening "$api_port")" ] && echo up || echo down)"
	echo "web  :$web_port $([ -n "$(listening "$web_port")" ] && echo up || echo down)"
}

case "${1:-}" in
start) start ;;
stop) stop ;;
restart) stop; start ;;
status) status ;;
*) echo "usage: $0 start|stop|restart|status" >&2; exit 2 ;;
esac
