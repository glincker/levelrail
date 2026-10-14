#!/usr/bin/env bash
# Re-seeds the demo data dir, and restores it when the tripwire fires.
# Usage: reseed.sh reseed | tripwire | hash <data-dir>
set -euo pipefail

ENV_FILE="${DEMO_ENV_FILE:-/etc/levelrail-demo/env}"
STATE_DIR="${DEMO_STATE_DIR:-/var/lib/levelrail-demo}"
BIN="${DEMO_BIN:-/opt/levelrail-demo/bin/levelrail}"
UNIT="levelrail-demo.service"
RUN_USER="levelrail-demo"

if [ -f "$ENV_FILE" ]; then
	set -a
	# shellcheck disable=SC1090
	. "$ENV_FILE"
	set +a
fi
DEMO_SEED="${DEMO_SEED:-1}"
# Request lines allowed to be non-GET and 2xx: the demo allowlist (sign-in, validators).
ALLOW='path=/api/v1/(demo/(session|cta)|auth/(login|logout)|apps/[^ ]+/validate-spec|pipelines/validate|iam/(policies/validate|preview|policy-templates/))'
MARKER="$STATE_DIR/seed.sha256"

# Hashes only the tables a bypassed write gate would touch; last_login_at
# is left out because every sign-in legitimately moves it.
data_hash() {
	local db="$1/levelrail.db"
	if [ ! -f "$db" ]; then
		echo "missing"
		return 0
	fi
	{
		sqlite3 -readonly "$db" "SELECT * FROM apps ORDER BY 1;"
		sqlite3 -readonly "$db" "SELECT * FROM desired_databases ORDER BY 1;"
		sqlite3 -readonly "$db" "SELECT id,email,display_name,password_hash,is_first_user FROM users ORDER BY 1;"
	} | sha256sum | cut -d' ' -f1
}

reseed() {
	local data="$STATE_DIR/data" next="$STATE_DIR/data.next" old="$STATE_DIR/data.old"
	rm -rf "$next" "$old"
	install -d -m 0700 -o "$RUN_USER" -g "$RUN_USER" "$next"
	runuser -u "$RUN_USER" -- env APP_DATA_DIR="$next" "$BIN" demo seed --seed "$DEMO_SEED"
	local fresh
	fresh="$(data_hash "$next")"
	systemctl stop "$UNIT"
	if [ -d "$data" ]; then mv "$data" "$old"; fi
	mv "$next" "$data"
	echo "$fresh" >"$MARKER"
	systemctl start "$UNIT"
	rm -rf "$old"
	echo "reseeded: $fresh"
}

write_seen() {
	journalctl -u "$UNIT" --since "1 hour ago" --no-pager -o cat 2>/dev/null |
		grep -E 'method=(POST|PUT|PATCH|DELETE) .*status=2[0-9][0-9]' |
		grep -Ev "$ALLOW" || true
}

tripwire() {
	local expected current hits
	expected="$(cat "$MARKER" 2>/dev/null || echo none)"
	current="$(data_hash "$STATE_DIR/data")"
	hits="$(write_seen)"
	if [ "$expected" != "$current" ]; then
		echo "tripwire: data drift (expected $expected, got $current)" >&2
		reseed
	elif [ -n "$hits" ]; then
		echo "tripwire: non-GET request succeeded outside the allowlist" >&2
		echo "$hits" | head -5 >&2
		reseed
	else
		echo "tripwire: ok"
	fi
}

case "${1:-}" in
reseed) reseed ;;
tripwire) tripwire ;;
hash) data_hash "${2:?data dir required}" ;;
*)
	echo "usage: $0 reseed|tripwire|hash <data-dir>" >&2
	exit 2
	;;
esac
