#!/bin/sh
# Container entrypoint: seed a fresh temp data dir, then serve it read-only.
# With APP_DEMO_RESET_INTERVAL set (e.g. 30m) it exits after that time so the
# restart policy starts a fresh container and reseeds.
set -eu

BIN="${LEVELRAIL_BIN:-levelrail}"

# Accepts digits followed by h, m or s, at least 60 seconds; prints seconds.
to_seconds() {
	case "$1" in
	*[!0-9hms]* | "" | *[hms]*[hms]* | [hms]* | 0*) return 1 ;;
	*h) n=$((${1%h} * 3600)) ;;
	*m) n=$((${1%m} * 60)) ;;
	*s) n=${1%s} ;;
	*) return 1 ;;
	esac
	[ "$n" -ge 60 ] || return 1
	echo "$n"
}

secs=""
if [ -n "${APP_DEMO_RESET_INTERVAL:-}" ]; then
	if ! secs="$(to_seconds "$APP_DEMO_RESET_INTERVAL")"; then
		echo "container-entrypoint: APP_DEMO_RESET_INTERVAL must be digits followed by h, m or s and at least 60 seconds (for example 30m), got: $APP_DEMO_RESET_INTERVAL" >&2
		exit 2
	fi
fi

APP_DATA_DIR="$(mktemp -d "${TMPDIR:-/tmp}/levelrail-demo.XXXXXX")"
export APP_DATA_DIR
export APP_DEMO_MODE="${APP_DEMO_MODE:-1}"

pid=""
timer=""
sleeper=""
fired="$APP_DATA_DIR.fired"

# Trap is set before seeding so an early signal still cleans the temp dir.
# shellcheck disable=SC2329 # invoked through trap
stop_children() {
	[ -z "$pid" ] || kill "$pid" 2>/dev/null || true
	[ -z "$timer" ] || kill "$timer" 2>/dev/null || true
}
# shellcheck disable=SC2329 # invoked through trap
cleanup() {
	stop_children
	[ -z "$pid" ] || wait "$pid" 2>/dev/null || true
	rm -rf "$APP_DATA_DIR"
	rm -f "$fired"
}
trap 'stop_children' TERM INT
trap cleanup EXIT

"$BIN" demo seed --seed "${DEMO_SEED:-1}"

if [ -z "$secs" ]; then
	"$BIN" demo serve &
	pid=$!
else
	"$BIN" demo serve &
	pid=$!
	(
		sleep "$secs" &
		sleeper=$!
		# The subshell's own TERM kills its sleep child so none is orphaned.
		trap 'kill "$sleeper" 2>/dev/null; exit 0' TERM
		wait "$sleeper" || exit 0
		: >"$fired"
		kill "$pid" 2>/dev/null || true
	) &
	timer=$!
fi

# A trapped signal interrupts wait; loop until serve is really gone.
rc=0
while kill -0 "$pid" 2>/dev/null; do
	wait "$pid" && rc=0 || rc=$?
done
if [ -f "$fired" ]; then
	rc=0
fi
exit "$rc"
