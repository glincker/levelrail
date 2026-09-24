#!/usr/bin/env bash
# Idle-footprint benchmark: boots a real control plane in dev mode in a
# throwaway data dir, creates N suspended apps (no containers, no image
# pulls), and reports RSS, CPU and API latency at each N.
#
# Usage:
#   scripts/bench-idle.sh                 # N = 0 100 500, 60s sample window
#   BENCH_COUNTS="0 100" BENCH_SECONDS=20 scripts/bench-idle.sh
#
# Needs only bash, curl, ps, awk and the Go toolchain. Numbers are dev-mode,
# same-machine figures, see docs/performance.md for the caveats.

set -uo pipefail

repo_root="$(git rev-parse --show-toplevel)"
cd "$repo_root"

counts="${BENCH_COUNTS:-0 100 500}"
seconds="${BENCH_SECONDS:-60}"
requests="${BENCH_REQUESTS:-200}"
port="${BENCH_PORT:-$((20000 + RANDOM % 20000))}"
work="$(mktemp -d -t levelrail-bench.XXXXXX)"
url="http://127.0.0.1:$port"
auth="Authorization: Bearer dev-root-token"
server_pid=""

cleanup() {
	if [ -n "$server_pid" ]; then
		kill "$server_pid" 2>/dev/null
		wait "$server_pid" 2>/dev/null
	fi
	rm -rf "$work"
}
trap cleanup EXIT

echo "bench: building"
go build -o "$work/levelrail" ./cmd/levelrail || exit 1

APP_API_RATE_LIMIT_READ_RPM=10000000 APP_API_RATE_LIMIT_WRITE_RPM=10000000 \
	APP_DEV_MODE=1 APP_DEV_FIXTURES_FILE="$repo_root/dev-fixtures.yml" \
	APP_DATA_DIR="$work/data" APP_HTTP_ADDR="127.0.0.1:$port" APP_AGENT_ADDR="127.0.0.1:0" \
	APP_INGRESS_HTTP_ADDR="127.0.0.1:0" APP_INGRESS_HTTPS_ADDR="127.0.0.1:0" \
	"$work/levelrail" >"$work/server.log" 2>&1 &
server_pid=$!

ready=0
for _ in $(seq 1 60); do
	kill -0 "$server_pid" 2>/dev/null || break
	if curl -fsS -o /dev/null -H "$auth" "$url/api/v1/apps" 2>/dev/null; then
		ready=1
		break
	fi
	sleep 0.5
done
if [ "$ready" -ne 1 ]; then
	echo "bench: control plane did not come up" >&2
	tail -20 "$work/server.log" >&2
	exit 1
fi

# cpu_seconds prints the process's cumulative CPU time in seconds.
cpu_seconds() {
	ps -o time= -p "$server_pid" | awk '{n=split($1,a,":"); s=0; for(i=1;i<=n;i++) s=s*60+a[i]; print s}'
}

rss_mb() {
	ps -o rss= -p "$server_pid" | awk '{printf "%.1f", $1/1024}'
}

# percentiles reads one latency in seconds per line and prints p50 and p95 in ms.
percentiles() {
	sort -n | awk '{v[NR]=$1} END {
		if (NR==0) {print "n/a n/a"; exit}
		i50=int(NR*0.50); if (i50<1) i50=1
		i95=int(NR*0.95); if (i95<1) i95=1
		printf "%.2f %.2f", v[i50]*1000, v[i95]*1000
	}'
}

# latency prints "p50 p95" in ms for `requests` sequential GETs on one connection.
latency() {
	local target="$1" args=()
	for _ in $(seq 1 "$requests"); do args+=(-o /dev/null "$target"); done
	curl -s -H "$auth" -w '%{time_total}\n' "${args[@]}" | percentiles
}

created=0
create_up_to() {
	local want="$1" i
	[ "$want" -le "$created" ] && return
	for i in $(seq $((created + 1)) "$want"); do
		local name
		name="$(printf 'bench-%04d' "$i")"
		curl -fsS -o /dev/null -H "$auth" -H 'Content-Type: application/json' \
			-d "{\"name\":\"$name\",\"image\":\"bench.invalid/none:0\",\"port\":8080}" \
			"$url/api/v1/apps" || { echo "bench: create $name failed" >&2; exit 1; }
		curl -fsS -o /dev/null -X POST -H "$auth" "$url/api/v1/apps/$name/stop" ||
			{ echo "bench: stop $name failed" >&2; exit 1; }
	done
	created="$want"
}

echo
printf '%-6s %-9s %-9s %-8s | %s\n' "apps" "rss_mb" "cpu_pct" "sample" "latency p50/p95 ms"
for n in $counts; do
	create_up_to "$n"
	sleep 5
	cpu0="$(cpu_seconds)"
	sleep "$seconds"
	cpu1="$(cpu_seconds)"
	rss="$(rss_mb)"
	cpu_pct="$(awk -v a="$cpu0" -v b="$cpu1" -v s="$seconds" 'BEGIN {printf "%.2f", (b-a)/s*100}')"
	printf '%-6s %-9s %-9s %-8s |\n' "$n" "$rss" "$cpu_pct" "${seconds}s"
	one="$(printf 'bench-%04d' 1)"
	[ "$n" -eq 0 ] && one="none"
	for ep in "/api/v1/apps" "/api/v1/apps/$one" "/api/v1/system/status" "/api/v1/system/doctor" \
		"/api/v1/certificates" "/api/v1/deploys/failed?since=24h"; do
		[ "$one" = "none" ] && [ "$ep" = "/api/v1/apps/none" ] && continue
		read -r p50 p95 <<<"$(latency "$url$ep")"
		printf '       GET %-38s %s / %s\n' "$ep" "$p50" "$p95"
	done
done
