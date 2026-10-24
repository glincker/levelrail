#!/usr/bin/env bash
# Run the documented load and failover scenarios against an existing Levelrail
# server and write a dated markdown fragment for docs/benchmarks.md.
# Run it on the load generator machine, never on the server under test.
# It never provisions or destroys anything: it only SSHes in to read hardware
# facts, and drives the app and API through `levelrail-cli bench run`.
#
# Usage: scripts/bench/run-vps.sh [--dry-run] [--out DIR] [--skip a,b]
# Env (required): BENCH_HOST, BENCH_USER, BENCH_INSTALL_URL (control plane URL),
#   BENCH_TOKEN (API token), BENCH_APP_URL (fixture app through the ingress)
# Env (optional): BENCH_REPLICA_URL (one replica, for overhead and replica kill),
#   BENCH_DRAIN_NODE, BENCH_DRAIN_TARGET (node drain), BENCH_SSH_OPTS,
#   BENCH_CLI (levelrail-cli path), BENCH_RATES, BENCH_DURATION, BENCH_NOTE
set -euo pipefail

dry=0
out_dir="docs/benchmark-results"
skip=","
while [ $# -gt 0 ]; do
  case "$1" in
    --dry-run) dry=1 ;;
    --out) out_dir="${2:?--out needs a directory}"; shift ;;
    --skip) skip=",${2:?--skip needs a list},"; shift ;;
    -h | --help) sed -n '2,15p' "$0" | sed 's/^# \{0,1\}//'; exit 0 ;;
    *) echo "run-vps: unknown argument $1" >&2; exit 2 ;;
  esac
  shift
done

cli="${BENCH_CLI:-levelrail-cli}"
rates="${BENCH_RATES:-100 500 1000 2000 4000}"
step="${BENCH_DURATION:-60s}"
fail_rate="${BENCH_FAILOVER_RATE:-500}"
fail_dur="${BENCH_FAILOVER_DURATION:-90s}"
today="$(date -u +%Y-%m-%d)"

need() {
  local v
  for v in "$@"; do
    if [ -z "${!v:-}" ]; then
      if [ "$dry" = 1 ]; then echo "run-vps: note: $v is not set (required for a real run)" >&2
      else echo "run-vps: $v is required" >&2; exit 2; fi
    fi
  done
}
need BENCH_HOST BENCH_USER BENCH_INSTALL_URL BENCH_TOKEN BENCH_APP_URL

host="${BENCH_HOST:-HOST}"
target="${BENCH_USER:-USER}@${host}"
app="${BENCH_APP_URL:-https://bench.example.com/}"
api="${BENCH_INSTALL_URL:-https://levelrail.example.com}"

# Hardware is read from the server, not typed, so the row cannot drift from the box.
remote_hw='lscpu | sed -n "s/^Model name: *//p;s/^CPU(s): *//p" | head -2; free -h | sed -n "2p"; uname -sr; docker version --format "{{.Server.Version}}"'

ssh_cmd() {
  # shellcheck disable=SC2086
  ssh -o BatchMode=yes -o ConnectTimeout=10 ${BENCH_SSH_OPTS:-} "$target" "$@"
}

run_cli() {
  # $1 name, rest: bench flags. Token is passed by env, never printed.
  local name="$1"; shift
  if [ "$dry" = 1 ]; then
    echo "  $cli bench run --api-url $api --url $app $* --json > $out_dir/$today-vps-$name.json"
  else
    "$cli" bench run --api-url "$api" --token "$BENCH_TOKEN" \
      --url "$app" "$@" --json > "$out_dir/$today-vps-$name.json"
  fi
}

want() { [[ "$skip" != *",$1,"* ]]; }

echo "run-vps: $today target=$target app=$app api=$api dry_run=$dry"
[ "$dry" = 1 ] || mkdir -p "$out_dir"

echo "1. hardware capture over SSH"
cpu="" cores="" mem="" kernel="" docker=""
if [ "$dry" = 1 ]; then
  echo "  ssh $target '$remote_hw'"
else
  hw="$(ssh_cmd "$remote_hw")"
  cpu="$(sed -n 1p <<<"$hw")"; cores="$(sed -n 2p <<<"$hw")"
  mem="$(sed -n 3p <<<"$hw" | awk '{print $2}')"
  kernel="$(sed -n 4p <<<"$hw")"; docker="$(sed -n 5p <<<"$hw")"
fi
hwnote="${cpu}, ${cores} vCPU, ${mem} RAM, ${kernel}, Docker ${docker}, ${BENCH_NOTE:-1 node}"
[ "$dry" = 1 ] && hwnote="<cpu>, <vcpus> vCPU, <ram> RAM, <kernel>, Docker <version>, ${BENCH_NOTE:-1 node}"

overhead=()
[ -n "${BENCH_REPLICA_URL:-}" ] && overhead=(--direct-url "$BENCH_REPLICA_URL")

if want steady; then
  echo "2. steady rate sweep ($rates req/s, $step each)"
  for r in $rates; do
    run_cli "steady-$r" --rate "$r" --duration "$step" --hardware "$hwnote" ${overhead[@]+"${overhead[@]}"}
  done
fi

if want replica-kill; then
  if [ -n "${BENCH_REPLICA_URL:-}" ]; then
    echo "3. replica kill at $fail_rate req/s"
    run_cli replica-kill --rate "$fail_rate" --duration "$fail_dur" --hardware "$hwnote" \
      --kill-replica-url "${BENCH_REPLICA_URL%/}/__bench/die"
  else echo "3. replica kill: skipped, BENCH_REPLICA_URL not set"; fi
fi

if want node-drain; then
  if [ -n "${BENCH_DRAIN_NODE:-}" ] && [ -n "${BENCH_DRAIN_TARGET:-}" ]; then
    echo "4. node drain (stand-in for node loss; a hard power-off is manual, see the runbook)"
    run_cli node-drain --rate "$fail_rate" --duration 120s --hardware "$hwnote" \
      --drain-node "$BENCH_DRAIN_NODE" --drain-target "$BENCH_DRAIN_TARGET"
  else echo "4. node drain: skipped, BENCH_DRAIN_NODE and BENCH_DRAIN_TARGET not set"; fi
fi

if want cp-restart; then
  echo "5. control plane restart at $fail_rate req/s"
  run_cli cp-restart --rate "$fail_rate" --duration "$fail_dur" --hardware "$hwnote" --restart-control-plane
fi

frag="$out_dir/$today-vps-fragment.md"
echo "6. markdown fragment -> $frag"
if [ "$dry" = 1 ]; then
  echo "  jq renders every $out_dir/$today-vps-*.json into a table row; nothing is written"
  exit 0
fi

command -v jq >/dev/null || { echo "run-vps: jq is required to render the fragment" >&2; exit 2; }
{
  echo "### Linux server run ($today)"
  echo
  echo "Hardware: $hwnote. Load generator: separate machine ($(uname -sr)). Client saturated must be 0 in every row."
  echo
  echo "| Scenario | Rate | Achieved | Failed | p50 ms | p95 ms | p99 ms | Max ms | Overhead p99 ms | Dropped | Error window ms | Recovery ms | Saturated |"
  echo "| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |"
  for f in "$out_dir/$today"-vps-*.json; do
    case "$f" in *-fragment.md) continue ;; esac
    name="$(basename "$f" .json)"; name="${name#"$today"-vps-}"
    jq -r --arg n "$name" '"| \($n) | \(.rate_rps) | \(.ingress.achieved_rps|floor) | \(.ingress.failed) of \(.ingress.sent) | \(.ingress.p50_ms) | \(.ingress.p95_ms) | \(.ingress.p99_ms) | \(.ingress.max_ms) | \(.ingress_overhead.p99_ms // "n/a") | \(.event.dropped_requests // "n/a") | \(.event.error_window_ms // "n/a") | \(.event.recovery_ms // "n/a") | \(.ingress.client_saturated) |"' "$f"
  done
} > "$frag"
echo "run-vps: done. Review $frag, then paste it into docs/benchmarks.md (docs/benchmark-runbook.md)."
