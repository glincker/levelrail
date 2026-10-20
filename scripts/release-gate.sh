#!/usr/bin/env bash
# Release gate: refuses to publish a release that cannot upgrade the one before it.
#   1. the release manifest names this tag and a real schema version
#   2. the candidate's schema is not lower than the previous release's
#   3. previous release -> candidate boots healthy on the same data and ends at schema head
#   4. the candidate refuses to start on a database newer than itself (exit 78)
# Hermetic: no Docker, only the control plane's own /readyz. Needs jq, sqlite3, curl, python3.
set -euo pipefail

candidate="" tag="" manifest="" prev_bin="" prev_tag="" prev_schema="" fixtures="" brand="" ready_timeout=90
repo_root="$(cd "$(dirname "$0")/.." && pwd)"

usage() {
	cat >&2 <<'EOF'
usage: release-gate.sh --candidate BIN --tag TAG [--manifest FILE] [--prev-binary BIN --prev-tag TAG [--prev-schema N]]
                       [--fixtures DIR] [--brand FILE] [--ready-timeout SECONDS]
EOF
	exit 2
}

while [ $# -gt 0 ]; do
	[ $# -ge 2 ] || usage
	case "$1" in
	--candidate) candidate="$2" ;;
	--tag) tag="$2" ;;
	--manifest) manifest="$2" ;;
	--prev-binary) prev_bin="$2" ;;
	--prev-tag) prev_tag="$2" ;;
	--prev-schema) prev_schema="$2" ;;
	--fixtures) fixtures="$2" ;;
	--brand) brand="$2" ;;
	--ready-timeout) ready_timeout="$2" ;;
	*) usage ;;
	esac
	shift 2
done
[ -n "$candidate" ] && [ -n "$tag" ] || usage
[ -n "$brand" ] || brand="$repo_root/brand.yaml"
for tool in jq sqlite3 curl python3; do
	command -v "$tool" >/dev/null 2>&1 || { echo "release-gate: $tool is required" >&2; exit 2; }
done

work="$(mktemp -d)"
pids=""
cleanup() {
	for p in $pids; do kill -9 "$p" 2>/dev/null || true; done
	rm -rf "$work"
}
trap cleanup EXIT

annotate() { # level message
	if [ "${GITHUB_ACTIONS:-}" = "true" ]; then echo "::$1::$2"; else echo "$1: $2" >&2; fi
}
warn() { annotate warning "$1"; }
fail() { annotate error "$1"; exit 1; }

schema_of() { "$1" version --json 2>/dev/null | jq -er '.schema_version' 2>/dev/null || true; }
db_schema() { sqlite3 "$1" 'SELECT COALESCE(MAX(version), 0) FROM schema_migrations'; }

free_port() { python3 -c 'import socket; s = socket.socket(); s.bind(("127.0.0.1", 0)); print(s.getsockname()[1])'; }

# boot BIN DATA_DIR LOG: starts the control plane in the background, waits for /readyz.
boot() {
	local bin="$1" data="$2" log="$3" port pid waited=0
	port="$(free_port)"
	APP_BRAND_FILE="$brand" APP_DEV_MODE=1 APP_DATA_DIR="$data" APP_HTTP_ADDR="127.0.0.1:$port" APP_AGENT_ADDR=127.0.0.1:0 \
		APP_INGRESS_HTTP_ADDR=127.0.0.1:0 APP_INGRESS_HTTPS_ADDR=127.0.0.1:0 "$bin" >"$log" 2>&1 &
	pid=$!
	pids="$pids $pid"
	while ! curl -fsS "http://127.0.0.1:$port/readyz" >/dev/null 2>&1; do
		if ! kill -0 "$pid" 2>/dev/null; then
			tail -n 30 "$log" >&2
			fail "$(basename "$bin") exited before /readyz answered (log above)"
		fi
		waited=$((waited + 1))
		if [ "$waited" -ge $((ready_timeout * 2)) ]; then
			tail -n 30 "$log" >&2
			fail "$(basename "$bin") did not answer /readyz within ${ready_timeout}s (log above)"
		fi
		sleep 0.5
	done
	BOOTED_PID="$pid"
}

stop() {
	local pid="$1" waited=0
	kill -TERM "$pid" 2>/dev/null || true
	while kill -0 "$pid" 2>/dev/null; do
		waited=$((waited + 1))
		if [ "$waited" -ge 60 ]; then
			kill -9 "$pid" 2>/dev/null || true
			warn "process $pid ignored SIGTERM for 30s and was killed"
			break
		fi
		sleep 0.5
	done
	wait "$pid" 2>/dev/null || true
}

[ -x "$candidate" ] || fail "candidate binary $candidate is not executable"
cand_schema="$(schema_of "$candidate")"
[ -n "$cand_schema" ] || fail "the candidate does not report its schema version (version --json)"

if [ -n "$manifest" ]; then
	jq -e --arg tag "$tag" --argjson schema "$cand_schema" '.version == $tag and .schema_version == $schema and .schema_version > 0' "$manifest" >/dev/null ||
		fail "release manifest does not describe $tag at schema $cand_schema: $(cat "$manifest")"
	echo "ok: manifest names $tag at schema $cand_schema"
fi

if [ -z "$prev_bin" ]; then
	warn "no previous release to compare against: the monotonic schema and upgrade smoke gates were skipped"
else
	[ -x "$prev_bin" ] || fail "previous binary $prev_bin is not executable"
	if [ -z "$prev_schema" ]; then
		prev_schema="$(schema_of "$prev_bin")"
	fi
	if [ -z "$prev_schema" ]; then
		warn "previous release ${prev_tag:-?} publishes no schema version and its binary does not report one: monotonic schema gate skipped"
	elif [ "$cand_schema" -lt "$prev_schema" ]; then
		fail "schema is not monotonic: $tag supports $cand_schema but ${prev_tag:-the previous release} already shipped $prev_schema, so installs of it could never upgrade to $tag"
	else
		echo "ok: schema $prev_schema (${prev_tag:-previous}) to $cand_schema ($tag) never goes backwards"
	fi

	data="$work/data"
	mkdir -p "$data"
	seeded="an empty data dir"
	if [ -z "$fixtures" ]; then
		fixtures="$work/fixtures"
		( cd "$repo_root" && CGO_ENABLED=0 scripts/upgrade-fixtures/generate.sh "$fixtures" >"$work/fixtures.log" 2>&1 ) ||
			{ tail -n 20 "$work/fixtures.log" >&2; fail "could not generate the upgrade fixtures"; }
	fi
	pick=""
	pick_version=-1
	for f in "$fixtures"/schema-*.db; do
		[ -f "$f" ] || continue
		v="$(basename "$f" .db)"
		v=$((10#${v#schema-}))
		if { [ -z "$prev_schema" ] || [ "$v" -le "$prev_schema" ]; } && [ "$v" -gt "$pick_version" ]; then
			pick="$f"
			pick_version="$v"
		fi
	done
	if [ -n "$pick" ]; then
		cp "$pick" "$data/levelrail.db"
		seeded="fixture schema $pick_version"
	else
		warn "no seeded fixture at or below schema ${prev_schema:-?}: the previous release creates the data itself"
	fi

	BOOTED_PID=""
	boot "$prev_bin" "$data" "$work/prev.log"
	stop "$BOOTED_PID"
	prev_db="$(db_schema "$data/levelrail.db")"
	[ -z "$prev_schema" ] || [ "$prev_db" = "$prev_schema" ] ||
		fail "previous release ended at schema $prev_db but advertises $prev_schema"
	users_before="$(sqlite3 "$data/levelrail.db" 'SELECT COUNT(*) FROM users' 2>/dev/null || echo 0)"
	echo "ok: ${prev_tag:-previous} booted healthy on $seeded, database at schema $prev_db"

	boot "$candidate" "$data" "$work/cand.log"
	stop "$BOOTED_PID"
	cand_db="$(db_schema "$data/levelrail.db")"
	[ "$cand_db" = "$cand_schema" ] || fail "after upgrading, the database is at schema $cand_db but $tag is at $cand_schema"
	users_after="$(sqlite3 "$data/levelrail.db" 'SELECT COUNT(*) FROM users' 2>/dev/null || echo 0)"
	[ "$users_after" -ge "$users_before" ] || fail "upgrade lost user rows: $users_before before, $users_after after"
	echo "ok: ${prev_tag:-previous} to $tag upgraded in place, healthy at schema $cand_db"
fi

newer="$work/newer"
mkdir -p "$newer"
if [ -f "$work/data/levelrail.db" ]; then
	cp "$work/data/levelrail.db" "$newer/levelrail.db"
else
	boot "$candidate" "$newer" "$work/seed.log"
	stop "$BOOTED_PID"
fi
sqlite3 "$newer/levelrail.db" "INSERT INTO schema_migrations (version, name) VALUES ($((cand_schema + 1000)), 'from_the_future')"
set +e
APP_BRAND_FILE="$brand" APP_DEV_MODE=1 APP_DATA_DIR="$newer" APP_HTTP_ADDR=127.0.0.1:0 APP_AGENT_ADDR=127.0.0.1:0 \
	APP_INGRESS_HTTP_ADDR=127.0.0.1:0 APP_INGRESS_HTTPS_ADDR=127.0.0.1:0 "$candidate" >"$work/downgrade.log" 2>&1 &
down_pid=$!
pids="$pids $down_pid"
waited=0
while kill -0 "$down_pid" 2>/dev/null && [ "$waited" -lt 120 ]; do
	waited=$((waited + 1))
	sleep 0.5
done
if kill -0 "$down_pid" 2>/dev/null; then
	kill -9 "$down_pid" 2>/dev/null
	set -e
	fail "$tag kept running on a database newer than itself instead of refusing to start"
fi
wait "$down_pid"
code=$?
set -e
[ "$code" -eq 78 ] || { tail -n 20 "$work/downgrade.log" >&2; fail "$tag exited $code on a newer database, want 78 (downgrade refused)"; }
echo "ok: $tag refuses a database newer than itself (exit 78)"
echo "release gate passed for $tag"
