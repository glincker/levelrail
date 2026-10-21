#!/usr/bin/env bash
# Local `go test -short` that shards internal/api across several processes.
# internal/api is thousands of mostly serial tests, so one process leaves the
# machine idle; shards are packed by scripts/ci-shard-names.sh, the same split
# CI uses, and run from one compiled test binary.
#
# Usage: scripts/test-fast.sh [-n shards] [-r name-regex] [package...]
#   -n  shard count for internal/api (default min(ncpu-2, 6), env TEST_FAST_SHARDS)
#   -r  only internal/api tests whose names match this regex (e.g. '^(TestA|TestB)$')
#   package...  other packages to test in one plain process (default: only internal/api)
# Environment: TEST_FAST_TIMEOUT (default 30m), TEST_FAST_SHARDS.
set -uo pipefail

root="$(git rev-parse --show-toplevel)" || exit 2
cd "$root" || exit 2

api_pkg="github.com/GLINCKER/levelrail/internal/api"
ncpu="$(sysctl -n hw.ncpu 2>/dev/null || nproc 2>/dev/null || echo 4)"
shards="${TEST_FAST_SHARDS:-$((ncpu - 2))}"
[ "$shards" -gt 6 ] && shards=6
[ "$shards" -lt 1 ] && shards=1
api_re=""
while getopts "n:r:" opt; do
	case "$opt" in
	n) shards="$OPTARG" ;;
	r) api_re="$OPTARG" ;;
	*) exit 2 ;;
	esac
done
shift $((OPTIND - 1))

run_api=0
others=()
if [ "$#" -eq 0 ]; then
	run_api=1
else
	for p in "$@"; do
		if [ "$p" = "$api_pkg" ] || [ "$p" = "./internal/api" ]; then
			run_api=1
		else
			others+=("$p")
		fi
	done
fi

timeout="${TEST_FAST_TIMEOUT:-30m}"
work="$(mktemp -d -t levelrail-test-fast.XXXXXX)"
ok=0
# Work dir (shard logs) is kept on failure so failing test names survive.
trap '[ "$ok" -eq 1 ] && rm -rf "$work"' EXIT
start="$(date +%s)"
pids=()
labels=()

if [ "${#others[@]}" -gt 0 ]; then
	mkdir -p "$work/tmp-other"
	(TMPDIR="$work/tmp-other" go test -short -timeout "$timeout" "${others[@]}" >"$work/other.log" 2>&1) &
	pids+=("$!")
	labels+=("other packages")
fi

if [ "$run_api" -eq 1 ]; then
	if ! go test -c -o "$work/api.test" ./internal/api 2>"$work/compile.log"; then
		cat "$work/compile.log"
		exit 1
	fi
	names="$(cd internal/api && "$work/api.test" -test.list '.*' | grep -E '^(Test|Example|Fuzz)' | sort)"
	# Same quarantine skip as scripts/ci-go-test.sh.
	if [ -f .github/flaky-tests.txt ]; then
		while read -r qpkg qtest _; do
			case "$qpkg" in '' | '#'*) continue ;; esac
			[ "$qpkg" = "$api_pkg" ] || continue
			names="$(grep -vxF "$qtest" <<<"$names" || true)"
		done <.github/flaky-tests.txt
	fi
	[ -n "$api_re" ] && names="$(grep -E "$api_re" <<<"$names" || true)"
	count="$(grep -c . <<<"$names" || true)"
	if [ "${count:-0}" -eq 0 ]; then
		echo "test-fast: no internal/api tests selected"
	else
		# Below ~40 tests per shard, process start and the sqlite warmup cost more than they save.
		max_useful=$(((count + 39) / 40))
		[ "$shards" -gt "$max_useful" ] && shards="$max_useful"
		i=1
		while [ "$i" -le "$shards" ]; do
			mine="$(scripts/ci-shard-names.sh "$i" "$shards" <<<"$names" | paste -sd'|' -)"
			if [ -n "$mine" ]; then
				mkdir -p "$work/tmp-$i"
				(cd internal/api && TMPDIR="$work/tmp-$i" "$work/api.test" -test.short -test.timeout "$timeout" -test.run "^($mine)\$" >"$work/api-$i.log" 2>&1) &
				pids+=("$!")
				labels+=("internal/api shard $i/$shards")
			fi
			i=$((i + 1))
		done
		echo "test-fast: internal/api, $count tests over $shards process(es)"
	fi
fi

report_failures() {
	local log="$1"
	{
		grep -E '^[[:space:]]*--- FAIL' "$log"
		grep -E '^(panic:|FAIL|fatal error:|.*test timed out)' "$log"
		awk '/^running tests:/{f=1;next} f&&/^\t/{print "stuck: " $0;next} {f=0}' "$log"
	} | sort -u | head -n 60
}

status=0
idx=0
failed_logs=""
for pid in "${pids[@]+"${pids[@]}"}"; do
	if ! wait "$pid"; then
		status=1
		echo "FAILED: ${labels[$idx]}"
		case "${labels[$idx]}" in
		"other packages") log="$work/other.log" ;;
		*) log="$work/api-$(sed -E 's/.*shard ([0-9]+)\/.*/\1/' <<<"${labels[$idx]}").log" ;;
		esac
		report_failures "$log"
		echo "  log: $log"
		failed_logs="$failed_logs ${labels[$idx]};"
	fi
	idx=$((idx + 1))
done

[ -f "$work/other.log" ] && [ "$status" -eq 0 ] && cat "$work/other.log"
elapsed=$(($(date +%s) - start))
if [ "$status" -eq 0 ]; then
	ok=1
	echo "test-fast: passed in ${elapsed}s"
else
	echo "test-fast: FAILED in ${elapsed}s (${failed_logs# }) logs kept in $work"
fi
exit "$status"
