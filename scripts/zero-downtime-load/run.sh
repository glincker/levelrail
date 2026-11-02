#!/usr/bin/env bash
# Hits an app URL at a steady rate while K redeploys run, then reports requests,
# non-2xx or failed count, latency percentiles and the longest gap between 2xx.
# Starts its own throwaway dev-mode control plane (never the dev app on :8080).
# Redeploys alternate the image tag (whoami v1.10.3 -> v1.10.2 -> v1.10.1), max 3 deploys.
# Usage: scripts/zero-downtime-load/run.sh [-r RATE] [-d DEPLOYS] [-s SETTLE_SECONDS]
# Env: ZDL_DIR (/private/tmp/claude-501/zd-load), ZDL_API_PORT (18480), ZDL_INGRESS_PORT (18482, HTTPS, local CA),
# ZDL_ENV (extra KEY=VAL for the control plane), ZDL_IMAGE (traefik/whoami).
set -uo pipefail
repo_root="$(git rev-parse --show-toplevel)"
dir="${ZDL_DIR:-/private/tmp/claude-501/zd-load}"
api="http://127.0.0.1:${ZDL_API_PORT:-18480}"
ing="https://zdl.localhost:${ZDL_INGRESS_PORT:-18482}"
res="zdl.localhost:${ZDL_INGRESS_PORT:-18482}:127.0.0.1"
tags=(v1.10.3 v1.10.2 v1.10.1 v1.10.0)
image="${ZDL_IMAGE:-traefik/whoami}"
export DOCKER_HOST="${DOCKER_HOST:-$(docker context inspect --format "{{.Endpoints.docker.Host}}" 2>/dev/null)}"
rate=20 deploys=$((${#tags[@]} - 1)) settle=8
while getopts r:d:s: o; do case $o in r) rate=$OPTARG ;; d) deploys=$OPTARG ;; s) settle=$OPTARG ;; *) exit 2 ;; esac; done
[ "$deploys" -ge 1 ] && [ "$deploys" -le $((${#tags[@]} - 1)) ] || { echo "-d must be between 1 and $((${#tags[@]} - 1)), one per extra image tag" >&2; exit 2; }
export GOFLAGS="${GOFLAGS:--buildvcs=false}"
tok="Authorization: Bearer dev-root-token"
host=zdl.localhost
mkdir -p "$dir/data"
(cd "$repo_root" && go build -o "$dir/levelrail" ./cmd/levelrail) || exit 1

# shellcheck disable=SC2086
env APP_DEV_MODE=1 APP_DATA_DIR="$dir/data" APP_HTTP_ADDR="127.0.0.1:${ZDL_API_PORT:-18480}" APP_AGENT_ADDR="127.0.0.1:0" \
	APP_INGRESS_HTTP_ADDR="127.0.0.1:0" APP_INGRESS_HTTPS_ADDR="127.0.0.1:${ZDL_INGRESS_PORT:-18482}" \
	APP_DEV_FIXTURES_FILE="$repo_root/test/fixtures/dev-fixtures.yml" ${ZDL_ENV:-} "$dir/levelrail" >"$dir/server.log" 2>&1 &
cp_pid=$!
loadpid=""
cleanup() {
	[ -n "$loadpid" ] && kill "$loadpid" 2>/dev/null
	curl -s -X DELETE -H "$tok" "$api/api/v1/apps/zdl-app" >/dev/null 2>&1
	kill "$cp_pid" 2>/dev/null; wait "$cp_pid" 2>/dev/null
	docker ps -aq --filter name=zdl- | xargs docker rm -f >/dev/null 2>&1
}
trap cleanup EXIT
for _ in $(seq 1 120); do curl -fsS -o /dev/null "$api/api/v1/brand" 2>/dev/null && break; sleep 0.5; done

spec() { printf '{"name":"zdl-app","image":"%s:%s","port":80,"domains":["%s"],"replicas":1,"strategy":"rolling"}' "$image" "${tags[$1]}" "$host"; }
call() { curl -s -o /dev/null -w '%{http_code}' -X "$1" -H "$tok" -H 'Content-Type: application/json' "${@:3}" "$api$2"; }
echo "create: $(call POST /api/v1/apps -d "$(spec 0)")"
host_of() { curl -sk -m 3 --resolve "$res" "$ing/" | sed -n 's/^Hostname: //p'; }
wait_new() { local prev=$1 h _; for _ in $(seq 1 360); do h=$(host_of); [ -n "$h" ] && [ "$h" != "$prev" ] && return 0; sleep 0.5; done; return 1; }
wait_new "" || { echo "app never answered"; tail -20 "$dir/server.log"; exit 1; }

log="$dir/load.log"; : >"$log"
(
	interval=$(awk -v r="$rate" 'BEGIN{printf "%.4f", 1/r}')
	while :; do
		# One request per tick in the background so a stall does not slow the schedule.
		( curl -sk -o /dev/null -m 5 --resolve "$res" -w "$(date +%s.%N) %{http_code} %{time_total}\n" "$ing/" >>"$log" 2>/dev/null || echo "$(date +%s.%N) 000 5" >>"$log" ) &
		sleep "$interval"
	done
) &
loadpid=$!
sleep 3
stuck=0
for n in $(seq 1 "$deploys"); do
	prev=$(host_of)
	echo "deploy $n: $(call POST /api/v1/apps/zdl-app/deploys -d "{\"image\":\"$image:${tags[$n]}\"}")"
	wait_new "$prev" && echo "deploy $n: new release answering" || { echo "deploy $n: NEVER CUT OVER"; stuck=$((stuck + 1)); }
	sleep "$settle"
done
kill "$loadpid" 2>/dev/null; wait "$loadpid" 2>/dev/null; loadpid=""
sleep 6

# macOS awk has no asort, so percentiles come from sort.
n=$(wc -l <"$log" | tr -d ' ')
bad=$(awk '$2 !~ /^2/' "$log" | wc -l | tr -d ' ')
ok=$((n - bad))
pct() { awk '$2 ~ /^2/ {print $3}' "$log" | sort -n | awk -v q="$1" -v t="$ok" 'NR==int(t*q)+1 {printf "%.0f", $1*1000}'; }
echo "requests $n"
echo "failures $bad"
echo "latency p50 $(pct 0.5)ms p95 $(pct 0.95)ms p99 $(pct 0.99)ms"
sort -n "$log" | awk '$2 ~ /^2/ { if (last && $1-last>gap) gap=$1-last; last=$1 } END { printf "longest gap between 2xx %.3fs\n", gap }'
if [ "$bad" -gt 0 ]; then
	echo "failing window: $(awk '$2 !~ /^2/ {print $1}' "$log" | sort -n | sed -n '1p;$p' | tr '\n' ' ')"
	awk '$2 !~ /^2/ {c[$2]++} END {for (k in c) print "  " c[k] " x status " k}' "$log"
fi
[ "$bad" -eq 0 ] && [ "$stuck" -eq 0 ]
rc=$?
[ "$stuck" -eq 0 ] || echo "$stuck deploy(s) never cut over, so this run proves nothing"
echo "control plane log: $dir/server.log"
exit "$rc"
