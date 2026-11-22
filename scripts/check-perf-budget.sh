#!/usr/bin/env bash
# Static performance budgets from scripts/perf-budget.txt: embedded web asset size
# (only when web/dist is built) and the committed idle footprint baseline.
# Chunk budgets live in web/bundle-budgets.json, not here.
# Usage: scripts/check-perf-budget.sh
set -euo pipefail
cd "$(git rev-parse --show-toplevel)"

budget=scripts/perf-budget.txt
baseline=scripts/idle-footprint-baseline.json
fail=0

max_of() { awk -v k="$1" '$1 == k { print $2 }' "$budget"; }

# check <key> <value>: fails when value exceeds the budget for key.
check() {
	local max
	max="$(max_of "$1")"
	[ -n "$max" ] || return 0
	if awk -v v="$2" -v m="$max" 'BEGIN { exit !(v > m) }'; then
		echo "perf budget exceeded: $1 is $2 (budget $max)"
		fail=1
	fi
}

if [ -d web/dist ]; then
	check web_dist_mb "$(du -sk web/dist | awk '{ printf "%.1f", $1 / 1024 }')"
	for ext in js css svg; do
		kb="$(find web/dist -type f -name "*.$ext" -exec wc -c {} + 2>/dev/null | awk '$2 == "total" { t = $1 } END { printf "%.0f", t / 1024 }')"
		check "embedded_${ext}_kb" "${kb:-0}"
	done
fi

if [ -f "$baseline" ] && command -v jq >/dev/null 2>&1; then
	# Worst row across the baseline scenarios.
	for pair in rss_mb:idle_rss_mb goroutines:idle_goroutines loops:idle_loops cpu_percent:idle_cpu_pct; do
		field="${pair%%:*}"
		key="${pair##*:}"
		check "$key" "$(jq -r "[.[].$field] | max" "$baseline")"
	done
fi

[ "$fail" -eq 0 ] && echo "perf budget: ok"
exit "$fail"
