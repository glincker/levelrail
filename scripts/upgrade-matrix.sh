#!/usr/bin/env bash
# Upgrade matrix: boots old releases on a private data dir with a bootstrap admin and a few API-created rows, then boots the new
# binary on the same data and checks it comes up. Run by the owner before cutting a release.
#
# Usage: scripts/upgrade-matrix.sh [--from N] [--pre] [--binary PATH] [--only-extra]
#                                  [--extra LABEL=PATH]
# Sources: v0.7.0 (mandatory), latest 0.6.x, plus the N latest stable releases (default 3).
# Env: UM_FAKE_SCHEMA=N inserts a fake schema_migrations row N into each old database (negative control).
# Linux hosts download the release binary (checksum verified); other hosts build the tag from source.
# Needs: gh, curl, sqlite3, go (for the new binary and non-linux hosts). Works on bash 3.2.

repo_root="$(git rev-parse --show-toplevel)"
from=3
pre=0
new_bin=""
only_extra=0
extras=()
mandatory="v0.7.0"
repo_slug="glincker/levelrail"
um_pass="UpgradeMatrix-Pass-12345"

while [ $# -gt 0 ]; do
	case "$1" in
	--from) from="$2"; shift 2 ;;
	--pre) pre=1; shift ;;
	--binary) new_bin="$2"; shift 2 ;;
	--only-extra) only_extra=1; shift ;;
	--extra) extras+=("$2"); shift 2 ;;
	-h | --help) sed -n 2,11p "$0"; exit 0 ;;
	*) echo "unknown flag $1" >&2; exit 2 ;;
	esac
done

work="$(mktemp -d "${TMPDIR:-/tmp}/upgrade-matrix.XXXXXX")"
pids=""
cleanup() {
	local p
	for p in $pids; do kill "$p" 2>/dev/null; done
}
trap cleanup EXIT

die() { echo "upgrade-matrix: $*" >&2; exit 2; }
for tool in curl sqlite3 go; do command -v "$tool" >/dev/null || die "missing $tool"; done

port_free() { ! lsof -iTCP:"$1" -sTCP:LISTEN -t >/dev/null 2>&1; }
pick_port() {
	local p i
	for i in $(seq 1 50); do
		p=$((20000 + RANDOM % 20000))
		case "$p" in 8080 | 5173) continue ;; esac
		port_free "$p" && { echo "$p"; return; }
	done
	die "no free port"
}

host_os="$(uname -s | tr '[:upper:]' '[:lower:]')"
host_arch="$(uname -m)"
case "$host_arch" in x86_64) host_arch=amd64 ;; aarch64 | arm64) host_arch=arm64 ;; esac

if [ -z "$new_bin" ]; then
	new_bin="$work/new-levelrail"
	echo "building working tree binary"
	(cd "$repo_root" && CGO_ENABLED=0 go build -o "$new_bin" ./cmd/levelrail) || die "build failed"
fi
[ -x "$new_bin" ] || die "new binary $new_bin not executable"

max_migration="$(ls "$repo_root/internal/store/migrations" | sed 's/_.*//' | sort -n | tail -1 | sed 's/^0*//')"

sha_check() {
	if command -v sha256sum >/dev/null; then (cd "$1" && sha256sum -c "$2" --ignore-missing >/dev/null 2>&1)
	else
		local want got
		want="$(grep " $3\$" "$1/$2" | awk '{print $1}')"
		got="$(shasum -a 256 "$1/$3" | awk '{print $1}')"
		[ -n "$want" ] && [ "$want" = "$got" ]
	fi
}

# fetch_old TAG -> prints binary path or returns 1 with reason in $work/TAG.fetch.err
fetch_old() {
	local tag="$1" dir="$work/bin-$1" asset
	mkdir -p "$dir"
	if [ "$host_os" = linux ]; then
		asset="levelrail-linux-$host_arch"
		gh release download "$tag" -R "$repo_slug" -D "$dir" -p "$asset" >/dev/null 2>"$work/$tag.fetch.err" || return 1
		if gh release download "$tag" -R "$repo_slug" -D "$dir" -p checksums.txt >/dev/null 2>&1; then
			sha_check "$dir" checksums.txt "$asset" || { echo "checksum mismatch for $asset" >"$work/$tag.fetch.err"; return 1; }
		fi
		chmod +x "$dir/$asset"
		echo "$dir/$asset"
		return 0
	fi
	git -C "$repo_root" rev-parse -q --verify "refs/tags/$tag" >/dev/null || git -C "$repo_root" fetch -q origin "refs/tags/$tag:refs/tags/$tag" 2>"$work/$tag.fetch.err" || return 1
	mkdir -p "$dir/src"
	git -C "$repo_root" archive "$tag" | tar -x -C "$dir/src" 2>"$work/$tag.fetch.err" || return 1
	(cd "$dir/src" && CGO_ENABLED=0 go build -o "$dir/levelrail" ./cmd/levelrail) >"$work/$tag.fetch.err" 2>&1 || return 1
	echo "$dir/levelrail"
}

start_bin() { # bin datadir port log
	(
		APP_ADMIN_USERNAME=admin APP_ADMIN_PASSWORD="$um_pass" \
			APP_DATA_DIR="$2" APP_HTTP_ADDR="127.0.0.1:$3" APP_AGENT_ADDR="127.0.0.1:0" \
			APP_INGRESS_HTTP_ADDR="127.0.0.1:0" APP_INGRESS_HTTPS_ADDR="127.0.0.1:0" \
			exec "$1" >"$4" 2>&1
	) &
	pids="$pids $!"
	LAST_PID=$!
}

wait_health() { # port pid
	local i
	for i in $(seq 1 120); do
		kill -0 "$2" 2>/dev/null || return 1
		[ "$(curl -s -o /dev/null -w '%{http_code}' "http://127.0.0.1:$1/healthz")" = 200 ] && return 0
		sleep 0.5
	done
	return 1
}

stop_pid() {
	local i
	kill -TERM "$1" 2>/dev/null
	for i in $(seq 1 40); do kill -0 "$1" 2>/dev/null || return 0; sleep 0.5; done
	kill -KILL "$1" 2>/dev/null
	return 1
}

api_ok() { # port jar -> 0 when login and apps list succeed
	local code
	code="$(curl -s -b "$2" -c "$2" -o /dev/null -w '%{http_code}' -H 'Content-Type: application/json' -d "{\"username\":\"admin\",\"password\":\"$um_pass\"}" "http://127.0.0.1:$1/api/v1/auth/login")"
	[ "$code" = 200 ] || { echo "admin login returned $code"; return 1; }
	code="$(curl -s -b "$2" -o /dev/null -w '%{http_code}' "http://127.0.0.1:$1/api/v1/apps")"
	[ "$code" = 200 ] || { echo "GET /api/v1/apps returned $code"; return 1; }
}

# Release binaries have no dev fixtures, so best-effort create a few rows over the API.
seed_rows() {
	local n
	for n in 1 2 3; do
		curl -s -b "$2" -o /dev/null -H 'Content-Type: application/json' \
			-d "{\"name\":\"um-app-$n\",\"image\":\"nginx:alpine\"}" "http://127.0.0.1:$1/api/v1/apps"
	done
}

# run_case LABEL OLDBIN -> sets CASE_REASON ("" on pass) and CASE_LOG
run_case() {
	local label="$1" old="$2" d port jar
	d="$work/case-$label"
	mkdir -p "$d/data"
	jar="$d/jar"
	CASE_REASON=""
	CASE_LOG="$d"

	port="$(pick_port)"
	start_bin "$old" "$d/data" "$port" "$d/old.log"
	if ! wait_health "$port" "$LAST_PID"; then
		CASE_REASON="old binary did not become healthy (see old.log)"
		stop_pid "$LAST_PID"
		return
	fi
	CASE_REASON="$(api_ok "$port" "$jar")" || { stop_pid "$LAST_PID"; return; }
	CASE_REASON=""
	seed_rows "$port" "$jar"
	stop_pid "$LAST_PID" || { CASE_REASON="old binary did not stop cleanly"; return; }

	if [ -n "${UM_FAKE_SCHEMA:-}" ]; then
		sqlite3 "$d/data/levelrail.db" "INSERT INTO schema_migrations(version,name) VALUES ($UM_FAKE_SCHEMA,'fake');" || { CASE_REASON="could not inject fake schema"; return; }
	fi

	port="$(pick_port)"
	start_bin "$new_bin" "$d/data" "$port" "$d/new.log"
	if ! wait_health "$port" "$LAST_PID"; then
		CASE_REASON="new binary failed to boot or /healthz not 200: $(grep -iE 'error|refus|newer|schema' "$d/new.log" | head -1 | cut -c1-160)"
		stop_pid "$LAST_PID"
		return
	fi
	local got
	got="$(sqlite3 "$d/data/levelrail.db" 'SELECT MAX(version) FROM schema_migrations;' 2>&1)"
	if [ "$got" != "$max_migration" ]; then
		CASE_REASON="schema version $got, expected $max_migration"
	elif grep -iE 'level=ERROR|"level":"error"|^ERROR' "$d/new.log" | grep -iE 'migrat|schema' >/dev/null; then
		CASE_REASON="migration or schema ERROR in log: $(grep -iE 'level=ERROR|"level":"error"|^ERROR' "$d/new.log" | grep -iE 'migrat|schema' | head -1 | cut -c1-160)"
	else
		CASE_REASON="$(api_ok "$port" "$jar")" || true
	fi
	stop_pid "$LAST_PID"
}

tags=()
add_tag() {
	local t
	for t in "${tags[@]}"; do [ "$t" = "$1" ] && return; done
	tags+=("$1")
}

if [ "$only_extra" = 0 ]; then
	command -v gh >/dev/null || die "missing gh"
	listing="$(gh release list -R "$repo_slug" --limit 100 --json tagName,isPrerelease --jq '.[] | "\(.tagName) \(.isPrerelease)"')" || die "gh release list failed"
	stable="$(echo "$listing" | while read -r t p; do
		case "$t" in kit/*) continue ;; esac
		{ [ "$p" = true ] && [ "$pre" = 0 ]; } && continue
		echo "$t"
	done)"
	add_tag "$mandatory"
	latest06="$(echo "$stable" | grep '^v0\.6\.' | sort -t. -k3 -n | tail -1)"
	[ -z "$latest06" ] || add_tag "$latest06"
	n=0
	for t in $(echo "$stable"); do
		[ "$n" -ge "$from" ] && break
		add_tag "$t"
		n=$((n + 1))
	done
fi

labels=()
reasons=()
logs=()
results=()

record() {
	labels+=("$1")
	results+=("$2")
	reasons+=("$3")
	logs+=("$4")
}

for tag in "${tags[@]}"; do
	echo "== $tag -> new"
	if bin="$(fetch_old "$tag")"; then
		run_case "$tag" "$bin"
		if [ -z "$CASE_REASON" ]; then record "$tag" PASS "" "$CASE_LOG"; else record "$tag" FAIL "$CASE_REASON" "$CASE_LOG"; fi
	else
		record "$tag" FAIL "could not obtain release binary: $(head -c 160 "$work/$tag.fetch.err" | tr '\n' ' ')" "$work"
	fi
done
for e in "${extras[@]}"; do
	label="${e%%=*}"
	echo "== $label -> new"
	run_case "$label" "${e#*=}"
	if [ -z "$CASE_REASON" ]; then record "$label" PASS "" "$CASE_LOG"; else record "$label" FAIL "$CASE_REASON" "$CASE_LOG"; fi
done

echo
printf '%-14s %-6s %s\n' SOURCE RESULT "FIRST FAILURE (log dir)"
fails=0
i=0
while [ "$i" -lt "${#labels[@]}" ]; do
	if [ "${results[$i]}" = PASS ]; then
		printf '%-14s %-6s\n' "${labels[$i]}" PASS
	else
		fails=$((fails + 1))
		printf '%-14s %-6s %s (%s)\n' "${labels[$i]}" FAIL "${reasons[$i]}" "${logs[$i]}"
	fi
	i=$((i + 1))
done
echo "workdir: $work"
[ "$fails" = 0 ] || exit 1
