#!/usr/bin/env bash
# Runs ./internal/api -short in the same lanes CI uses (api-1..api-N, split by
# scripts/ci-shard-names.sh) with bounded concurrency. Prints failures only,
# then one summary line.
#
# Usage: scripts/gate-api.sh [-r name-regex]
# Env: GATE_API_LANES (default 4, CI's count), GATE_API_CONCURRENCY (default
# min(lanes, cpus-2)), GATE_API_TIMEOUT (default 25m).
set -uo pipefail

cd "$(git rev-parse --show-toplevel)" || exit 2

lanes="${GATE_API_LANES:-4}"
timeout="${GATE_API_TIMEOUT:-25m}"
ncpu="$(sysctl -n hw.ncpu 2>/dev/null || nproc 2>/dev/null || echo 4)"
conc="${GATE_API_CONCURRENCY:-$((ncpu - 2))}"
[ "$conc" -gt "$lanes" ] && conc="$lanes"
[ "$conc" -lt 1 ] && conc=1
filter=""
while getopts "r:" opt; do
	case "$opt" in
	r) filter="$OPTARG" ;;
	*) exit 2 ;;
	esac
done

work="$(mktemp -d -t levelrail-gate-api.XXXXXX)"
ok=0
# Logs stay on failure so the failing lane output can be read afterwards.
trap '[ "$ok" -eq 1 ] && rm -rf "$work"' EXIT
start="$(date +%s)"

if ! go test -c -o "$work/api.test" ./internal/api 2>"$work/compile.log"; then
	cat "$work/compile.log"
	echo "gate-api: compile failed"
	exit 1
fi
names="$(cd internal/api && "$work/api.test" -test.list '.*' | grep -E '^(Test|Example|Fuzz)' | sort)"
if [ -f .github/flaky-tests.txt ]; then
	while read -r qpkg qtest _; do
		case "$qpkg" in '' | '#'*) continue ;; esac
		[ "$qpkg" = "github.com/GLINCKER/levelrail/internal/api" ] || continue
		names="$(grep -vxF "$qtest" <<<"$names" || true)"
	done <.github/flaky-tests.txt
fi
[ -n "$filter" ] && names="$(grep -E "$filter" <<<"$names" || true)"

run_lane() {
	local i="$1" mine
	mine="$(scripts/ci-shard-names.sh "$i" "$lanes" <<<"$names" | paste -sd'|' -)"
	[ -n "$mine" ] || return 0
	mkdir -p "$work/tmp-$i"
	(cd internal/api && TMPDIR="$work/tmp-$i" "$work/api.test" -test.short -test.timeout "$timeout" -test.run "^($mine)\$" >"$work/api-$i.log" 2>&1)
}

pids=()
lane_of=()
failed=0
reap() {
	local pid="$1" lane="$2"
	if ! wait "$pid"; then
		failed=$((failed + 1))
		echo "FAILED: api-$lane (log: $work/api-$lane.log)"
		awk '/^[[:space:]]*--- FAIL/ {p=1; print; next} p && /^[[:space:]]/ {print; next} {p=0}' "$work/api-$lane.log" | head -n 80
		grep -E '^(panic:|FAIL|fatal error:|.*test timed out)' "$work/api-$lane.log" | head -n 10
	fi
}

running=0
for i in $(seq 1 "$lanes"); do
	if [ "$running" -ge "$conc" ]; then
		reap "${pids[0]}" "${lane_of[0]}"
		pids=("${pids[@]:1}")
		lane_of=("${lane_of[@]:1}")
		running=$((running - 1))
	fi
	run_lane "$i" &
	pids+=("$!")
	lane_of+=("$i")
	running=$((running + 1))
done
for idx in "${!pids[@]}"; do
	reap "${pids[$idx]}" "${lane_of[$idx]}"
done

elapsed=$(($(date +%s) - start))
if [ "$failed" -eq 0 ]; then
	ok=1
	echo "gate-api: PASS, $lanes lanes (concurrency $conc) in ${elapsed}s"
	exit 0
fi
echo "gate-api: FAIL, $failed of $lanes lanes failed in ${elapsed}s"
exit 1
