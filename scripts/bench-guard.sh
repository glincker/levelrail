#!/usr/bin/env bash
# Runs the scale-suspect Go benchmarks and compares them with the committed
# baseline (test/bench/baseline.json). Exits non-zero on a regression.
#
# Usage:
#   scripts/bench-guard.sh                      # compare
#   LEVELRAIL_BENCH_UPDATE=1 scripts/bench-guard.sh   # rewrite the baseline
#
# Knobs (env): LEVELRAIL_BENCH_TOLERANCE_PCT (default 25), LEVELRAIL_BENCH_COUNT,
# LEVELRAIL_BENCH_TIME, LEVELRAIL_BENCH_PATTERN, LEVELRAIL_BENCH_INFO_PATTERN.
# Wall time is compared only when the baseline came from the same CPU model;
# allocations are always compared.

set -euo pipefail

repo_root="$(git rev-parse --show-toplevel)"
cd "$repo_root"

tolerance="${LEVELRAIL_BENCH_TOLERANCE_PCT:-25}"
count="${LEVELRAIL_BENCH_COUNT:-3}"
benchtime="${LEVELRAIL_BENCH_TIME:-200ms}"
pattern="${LEVELRAIL_BENCH_PATTERN:-BuildRoutesConfig|BuildAndMarshalRoutesConfig|ListApps|V2WatchFanout}"
baseline="${LEVELRAIL_BENCH_BASELINE:-$repo_root/test/bench/baseline.json}"
out="$(mktemp -t levelrail-bench.XXXXXX)"
trap 'rm -f "$out"' EXIT

mkdir -p "$(dirname "$baseline")"

# -p 1 keeps packages from competing for CPU while one of them is timed.
go test -buildvcs=false -p 1 -run '^$' -bench "$pattern" -benchmem \
  -count "$count" -benchtime "$benchtime" -timeout 20m \
  ./internal/api ./internal/ingress 2>&1 | tee "$out" | grep -E '^(Benchmark|goos|goarch|cpu|FAIL|ok|---)' || true

if grep -q '^FAIL' "$out"; then
  echo "bench-guard: benchmarks failed to run" >&2
  exit 1
fi

LEVELRAIL_BENCH_OUTPUT="$out" LEVELRAIL_BENCH_BASELINE="$baseline" \
  LEVELRAIL_BENCH_TOLERANCE_PCT="$tolerance" \
  go test -buildvcs=false -count=1 -run '^TestGoBenchGuard$' -v ./internal/benchreport
