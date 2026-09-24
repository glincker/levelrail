#!/usr/bin/env bash
# Boots a real control plane from the current working tree in a throwaway
# data dir (dev mode, no Docker needed) and runs CLI commands against it,
# so a feature is proven by using it, not just by its unit tests.
#
# Usage:
#   scripts/smoke.sh -- nodes list
#   scripts/smoke.sh -- attention --json -- apps list
#
# Each "--" starts one CLI invocation. The CLI runs with the fixed dev-root-token
# token from dev-fixtures.yml. Exits non-zero if the server fails to come
# up or any command exits non-zero. Set SMOKE_KEEP=1 to leave the server
# running (prints its URL) for manual poking, or SMOKE_PORT to pick a port.
#
# Web-only change? Skip this and run: cd web && npx tsc --noEmit && npx vitest run --changed

set -uo pipefail

repo_root="$(git rev-parse --show-toplevel)"
cd "$repo_root"

port="${SMOKE_PORT:-$((20000 + RANDOM % 20000))}"
work="$(mktemp -d -t levelrail-smoke.XXXXXX)"
url="http://127.0.0.1:$port"
server_pid=""

cleanup() {
	if [ -n "$server_pid" ] && [ -z "${SMOKE_KEEP:-}" ]; then
		kill "$server_pid" 2>/dev/null
		wait "$server_pid" 2>/dev/null
	fi
	if [ -z "${SMOKE_KEEP:-}" ]; then
		rm -rf "$work"
	fi
}
trap cleanup EXIT

echo "smoke: building"
go build -o "$work/levelrail" ./cmd/levelrail || exit 1
go build -o "$work/levelrail-cli" ./cmd/levelrail-cli || exit 1

echo "smoke: starting control plane on $url"
APP_DEV_MODE=1 APP_DEV_FIXTURES_FILE="$repo_root/dev-fixtures.yml" \
	APP_DATA_DIR="$work/data" APP_HTTP_ADDR="127.0.0.1:$port" APP_AGENT_ADDR="127.0.0.1:0" \
	APP_INGRESS_HTTP_ADDR="127.0.0.1:0" APP_INGRESS_HTTPS_ADDR="127.0.0.1:0" \
	"$work/levelrail" >"$work/server.log" 2>&1 &
server_pid=$!

ready=0
for _ in $(seq 1 60); do
	if ! kill -0 "$server_pid" 2>/dev/null; then
		break
	fi
	if curl -fsS -o /dev/null -H "Authorization: Bearer dev-root-token" "$url/api/v1/apps" 2>/dev/null; then
		ready=1
		break
	fi
	sleep 0.5
done
if [ "$ready" -ne 1 ]; then
	echo "smoke: control plane did not come up, last log lines:" >&2
	tail -20 "$work/server.log" >&2
	exit 1
fi

if [ -n "${SMOKE_KEEP:-}" ]; then
	echo "smoke: server left running at $url (pid $server_pid, log $work/server.log)"
	echo "smoke: token: dev-root-token, CLI: $work/levelrail-cli --api-url $url --token dev-root-token <cmd>"
fi

rc=0
args=()
run_cmd() {
	[ "${#args[@]}" -eq 0 ] && return
	echo "smoke: levelrail-cli ${args[*]}"
	if ! "$work/levelrail-cli" "${args[@]}" --api-url "$url" --token dev-root-token; then
		echo "smoke: FAILED: ${args[*]}" >&2
		rc=1
	fi
	args=()
}
for a in "$@"; do
	if [ "$a" = "--" ]; then
		run_cmd
	else
		args+=("$a")
	fi
done
run_cmd

[ "$rc" -eq 0 ] && echo "smoke: all commands succeeded"
exit "$rc"
