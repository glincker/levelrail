#!/bin/sh
# Container entrypoint: seed a fresh temp data dir, then serve it read-only.
# With APP_DEMO_RESET_INTERVAL set (e.g. 30m) it exits after that time so the
# restart policy starts a fresh container and reseeds.
set -eu

BIN="${LEVELRAIL_BIN:-levelrail}"
APP_DATA_DIR="$(mktemp -d "${TMPDIR:-/tmp}/levelrail-demo.XXXXXX")"
export APP_DATA_DIR
export APP_DEMO_MODE="${APP_DEMO_MODE:-1}"

"$BIN" demo seed --seed "${DEMO_SEED:-1}"

# Converts 90s, 30m, 2h or bare seconds to seconds; empty means no reset.
to_seconds() {
	case "$1" in
	*h) echo $((${1%h} * 3600)) ;;
	*m) echo $((${1%m} * 60)) ;;
	*s) echo "${1%s}" ;;
	*) echo "$1" ;;
	esac
}

if [ -z "${APP_DEMO_RESET_INTERVAL:-}" ]; then
	exec "$BIN" demo serve
fi

secs="$(to_seconds "$APP_DEMO_RESET_INTERVAL")"
"$BIN" demo serve &
pid=$!
trap 'kill "$pid" 2>/dev/null' TERM INT
( sleep "$secs"; kill "$pid" 2>/dev/null ) &
timer=$!
wait "$pid" || true
kill "$timer" 2>/dev/null || true
rm -rf "$APP_DATA_DIR"
exit 0
