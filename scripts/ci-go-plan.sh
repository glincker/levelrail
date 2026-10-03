#!/usr/bin/env bash
# Turns a diff into ci.yml's Go work: the affected package set and the test
# lane matrix sized to it.
#
# Usage: scripts/ci-go-plan.sh <base> [head]   scope to base..head
#        scripts/ci-go-plan.sh --full          every package (./...)
# Output: key=value lines for $GITHUB_OUTPUT, a human summary on stderr.
#
#   scope        all | some | none
#   packages     space-separated import paths to build and vet ("./..." for all)
#   has_tests    whether the test matrix has any lane
#   matrix       {"include":[{lane, packages, shard, flags, cache, save_cache}]}
#   lanes        lane names, space-separated (coverage artifacts to expect)
#   api_checks   JSON array of check names Test (internal/api) waits on
#   rest_checks  JSON array of check names Test (everything else) waits on
#   coverage     none | changed (changed-line gate only) | full (aggregate too)
#   quarantine   whether the non-blocking quarantine lane has anything to run
#
# CI_SMALL_LANE_MAX (default 8): at or under this many non-api packages,
# docker-backed and plain packages share one lane instead of two.
set -euo pipefail

cd "$(git rev-parse --show-toplevel)"

small_max="${CI_SMALL_LANE_MAX:-8}"
api_pkg="github.com/GLINCKER/levelrail/internal/api"
api_shards=4
flaky_file=.github/flaky-tests.txt

scope=some
affected=()
flaky_changed=false
if [ "${1:-}" = "--full" ]; then
	scope=all
else
	base="${1:?usage: ci-go-plan.sh <base> [head] | --full}"
	head="${2:-HEAD}"
	mapfile -t files < <(git diff --name-only --diff-filter=ACDMR "$base" "$head")
	out="$(printf '%s\n' "${files[@]}" | scripts/affected-go-packages.sh --stdin)"
	if [ "$out" = "ALL" ]; then
		scope=all
	elif [ -n "$out" ]; then
		mapfile -t affected <<<"$out"
	fi
	# A quarantine entry added or removed changes what its package runs.
	if printf '%s\n' "${files[@]}" | grep -qxF "$flaky_file"; then
		flaky_changed=true
		while read -r pkg; do
			[ -n "$pkg" ] && affected+=("$pkg")
		done < <(git diff "$base" "$head" -- "$flaky_file" | sed -n 's/^[+-]\([^+#-][^ ]*\) .*/\1/p')
		mapfile -t affected < <(printf '%s\n' "${affected[@]}" | sed '/^$/d' | sort -u)
	fi
	if [ "$scope" != all ] && [ "${#affected[@]}" -eq 0 ]; then
		scope=none
	fi
fi

group() {
	if [ "$scope" = all ]; then
		scripts/go-test-groups.sh "$1"
	else
		scripts/go-test-groups.sh "$1" "${affected[@]}"
	fi
}

api=()
docker=()
rest=()
if [ "$scope" != none ]; then
	mapfile -t api < <(group api)
	mapfile -t docker < <(group docker)
	mapfile -t rest < <(group rest)
fi

entries=()
api_checks=()
rest_checks=()
lanes=()
lane() { # name packages shard flags cache save_cache
	entries+=("$(jq -nc --arg lane "$1" --arg packages "$2" --arg shard "$3" --arg flags "$4" \
		--arg cache "$5" --argjson save "$6" \
		'{lane: $lane, packages: $packages, shard: $shard, flags: $flags, cache: $cache, save_cache: $save}')")
	lanes+=("$1")
}
join() { local IFS=' '; echo "$*"; }

if [ "${#api[@]}" -gt 0 ]; then
	for i in $(seq 1 "$api_shards"); do
		save=false
		[ "$i" -eq 1 ] && save=true
		lane "api-$i" "$api_pkg" "$i/$api_shards" "-short -timeout=10m" api "$save"
		api_checks+=("Test (api-$i)")
	done
fi

other=$((${#docker[@]} + ${#rest[@]}))
# Docker-backed packages run real container lifecycles, not unit logic;
# timeout-minutes: 20 on the job already budgets for this, -timeout just
# needs to use more of it (test/e2e grew past 12m, see docs/ci.md).
if [ "$scope" != all ] && [ "$other" -gt 0 ] && [ "$other" -le "$small_max" ]; then
	lane rest "$(join "${docker[@]}" "${rest[@]}")" "" "-short -p 2 -timeout=18m" rest false
	rest_checks+=("Test (rest)")
else
	if [ "${#docker[@]}" -gt 0 ]; then
		lane docker "$(join "${docker[@]}")" "" "-short -p 2 -timeout=18m" docker true
		rest_checks+=("Test (docker)")
	fi
	if [ "${#rest[@]}" -gt 0 ]; then
		lane rest "$(join "${rest[@]}")" "" "-short -timeout=10m" rest true
		rest_checks+=("Test (rest)")
	fi
fi

quarantine=false
if [ "$flaky_changed" = true ]; then
	quarantine=true
elif [ -f "$flaky_file" ]; then
	while read -r qpkg _; do
		case "$qpkg" in '' | '#'*) continue ;; esac
		if [ "$scope" = all ] || printf '%s\n' "${affected[@]}" | grep -qxF "$qpkg"; then
			quarantine=true
		fi
	done <"$flaky_file"
fi

case "$scope" in
all) packages="./..." coverage=full ;;
some) packages="$(join "${affected[@]}")" coverage=changed ;;
none) packages="" coverage=none ;;
esac
has_tests=false
[ "${#entries[@]}" -gt 0 ] && has_tests=true

to_json_array() { if [ "$#" -eq 0 ]; then echo '[]'; else printf '%s\n' "$@" | jq -R . | jq -sc .; fi; }

echo "scope=$scope"
echo "packages=$packages"
echo "has_tests=$has_tests"
echo "matrix=$(if [ "${#entries[@]}" -eq 0 ]; then echo '{"include":[]}'; else printf '%s\n' "${entries[@]}" | jq -sc '{include: .}'; fi)"
echo "lanes=$(join "${lanes[@]}")"
echo "api_checks=$(to_json_array "${api_checks[@]}")"
echo "rest_checks=$(to_json_array "${rest_checks[@]}")"
echo "coverage=$coverage"
echo "quarantine=$quarantine"

{
	echo "go scope: $scope (${#affected[@]} affected package(s))"
	echo "lanes: ${lanes[*]:-none} (api ${#api[@]}, docker ${#docker[@]}, rest ${#rest[@]})"
	echo "coverage gate: $coverage, quarantine lane: $quarantine"
} >&2
