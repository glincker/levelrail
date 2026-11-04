#!/usr/bin/env bash
# Shared settings for the lab scripts. Source it, do not run it.
# Every tunable comes from the environment so nothing is fixed in the scripts.

lab_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
LAB_DIR="${LAB_DIR:-${TMPDIR:-/tmp}/levelrail-lab}"
LAB_PORT="${LAB_PORT:-18480}"
LAB_NODES="${LAB_NODES:-4}"
LAB_ZONES="${LAB_ZONES:-a,b}"
LAB_READY_TIMEOUT="${LAB_READY_TIMEOUT:-60}"
LAB_URL="http://127.0.0.1:${LAB_PORT}"

# Ports and folders that belong to a developer's own instance.
lab_forbidden_ports="8080 8180 5273 5173"

lab_guard() {
	local p
	for p in $lab_forbidden_ports; do
		[ "$LAB_PORT" != "$p" ] || { echo "lab: port $p belongs to a dev instance, pick another LAB_PORT" >&2; return 1; }
	done
	case "$LAB_DIR" in
	"$HOME/levelrail-local-data"* | /var/lib/levelrail* | "$HOME/.levelrail"*)
		echo "lab: LAB_DIR must not be an existing data dir ($LAB_DIR)" >&2
		return 1
		;;
	esac
	if [ -d "$LAB_DIR" ] && [ -n "$(ls -A "$LAB_DIR" 2>/dev/null)" ] && [ ! -f "$LAB_DIR/.lab" ]; then
		echo "lab: $LAB_DIR exists and is not a lab dir, refusing to use it" >&2
		return 1
	fi
}

lab_listening() { lsof -iTCP:"$1" -sTCP:LISTEN -t 2>/dev/null; }

lab_token() { cat "$LAB_DIR/token"; }

# lab_api METHOD PATH [JSON]: raw API call, prints the body, status on the last line.
lab_api() {
	local method="$1" path="$2" body="${3:-}"
	if [ -n "$body" ]; then
		curl -sS -X "$method" -H "Authorization: Bearer $(lab_token)" -H "Content-Type: application/json" \
			-w '\n%{http_code}' --data "$body" "$LAB_URL$path"
	else
		curl -sS -X "$method" -H "Authorization: Bearer $(lab_token)" -w '\n%{http_code}' "$LAB_URL$path"
	fi
}

# lab_cli ARGS: the real CLI against the lab control plane.
lab_cli() {
	HOME="$LAB_DIR/home" LEVELRAIL_API_URL="$LAB_URL" LEVELRAIL_TOKEN="$(lab_token)" "$LAB_DIR/bin/levelrail-cli" "$@"
}

# lab_login: signs in as the dev-mode admin (a human root session) into a cookie jar.
lab_login() {
	LAB_JAR="$LAB_DIR/session.jar"
	rm -f "$LAB_JAR"
	curl -fsS -o /dev/null -c "$LAB_JAR" -X POST -H "Content-Type: application/json" \
		--data '{"username":"dev","password":"dev"}' "$LAB_URL/api/v1/auth/login"
}

# lab_sess METHOD PATH [JSON] [HEADER]: like lab_api but as the human session.
lab_sess() {
	local method="$1" path="$2" body="${3:-}" hdr="${4:-X-Lab: 1}"
	curl -sS -X "$method" -b "$LAB_JAR" -H "$hdr" -H "Content-Type: application/json" -H "Origin: $LAB_URL" \
		-w '\n%{http_code}' ${body:+--data "$body"} "$LAB_URL$path"
}
