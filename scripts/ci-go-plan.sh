#!/usr/bin/env bash
# Turns a diff into ci.yml's Go work: the affected package set and the test
# lane matrix sized to it.
#
# Usage: scripts/ci-go-plan.sh <base> [head]   scope to base..head
#        scripts/ci-go-plan.sh --full          every package (./...)
#        scripts/ci-go-plan.sh --files         changed paths on stdin
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
#   live         full | smoke: size of the heavy live-Docker suites this PR runs
#   live_reason  why (the matched path, the label, or "no live-suite path changed")
#
# CI_LIVE_LABEL=true (the ci:live PR label) forces live=full. The heavy
# suites skip themselves under LEVELRAIL_LIVE_SUITE=smoke (test/e2e/testenv).
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
	if [ "${1:-}" = "--files" ]; then
		# Changed paths on stdin instead of a git diff (scripts/test-ci-go-plan.sh).
		base="" head=""
		mapfile -t files
	else
		base="${1:?usage: ci-go-plan.sh <base> [head] | --files | --full}"
		head="${2:-HEAD}"
		mapfile -t files < <(git diff --name-only --diff-filter=ACDMR "$base" "$head")
	fi
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
		done < <(if [ -n "$base" ]; then git diff "$base" "$head" -- "$flaky_file"; fi | sed -n 's/^[+-]\([^+#-][^ ]*\) .*/\1/p')
		mapfile -t affected < <(printf '%s\n' "${affected[@]}" | sed '/^$/d' | sort -u)
	fi
	if [ "$scope" != all ] && [ "${#affected[@]}" -eq 0 ]; then
		scope=none
	fi
fi

# Paths whose change can break a heavy live suite (docs/ci.md#live-suites).
live_paths_re='^(internal/(catalog|compose|registrycatalog|reconcile|docker|dockertest|agent|build|pipeline|ingress)/|test/e2e/|\.github/workflows/(ci|nightly)\.yml$|\.github/actions/|scripts/(ci-go-plan|ci-go-test|go-test-groups)\.sh$)'
live=smoke
live_reason="no live-suite path changed"
if [ "$scope" = all ]; then
	live=full
	live_reason="full run"
elif [ "${CI_LIVE_LABEL:-}" = true ]; then
	live=full
	live_reason="PR label ci:live"
else
	hit="$(printf '%s\n' "${files[@]}" | grep -E "$live_paths_re" | head -n1 || true)"
	if [ -n "$hit" ]; then
		live=full
		live_reason="diff touches $hit"
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
# All test lanes share one Go cache (their dependency compile is the bulk of
# it); the first lane planned saves it at job end, on push events only.
lane() { # name packages shard flags cache save_cache name_filter name_skip
	save_flag=false
	[ "${#entries[@]}" -eq 0 ] && save_flag=true
	set -- "$1" "$2" "$3" "$4" all "$save_flag" "${7:-}" "${8:-}"
	entries+=("$(jq -nc --arg lane "$1" --arg packages "$2" --arg shard "$3" --arg flags "$4" \
		--arg cache "$5" --argjson save "$6" --arg name_filter "${7:-}" --arg name_skip "${8:-}" --arg live "$live" \
		'{lane: $lane, packages: $packages, shard: $shard, flags: $flags, cache: $cache, save_cache: $save, name_filter: $name_filter, name_skip: $name_skip, live: $live}')")
	lanes+=("$1")
}
join() { local IFS=' '; echo "$*"; }

if [ "${#api[@]}" -gt 0 ]; then
	# api affected only via a dependency, not a direct file change:
	# scripts/affected-api-tests.sh can sometimes narrow the shards to
	# the tests that actually reach it, instead of the whole package.
	api_name_filter=""
	if [ "$scope" = some ] && ! printf '%s\n' "${files[@]}" | grep -q '^internal/api/'; then
		seed_dirs="$(printf '%s\n' "${files[@]}" | scripts/affected-go-packages.sh --stdin --seeds 2>/dev/null || true)"
		if [ -n "$seed_dirs" ]; then
			mapfile -t seed_dirs_arr <<<"$seed_dirs"
			mapfile -t seed_pkgs < <(go list -e -f '{{.ImportPath}}' "${seed_dirs_arr[@]}" 2>/dev/null | grep -vxF "$api_pkg" || true)
			if [ "${#seed_pkgs[@]}" -gt 0 ]; then
				narrowed="$(scripts/affected-api-tests.sh "${seed_pkgs[@]}" 2>/dev/null || echo ALL)"
				[ "$narrowed" != ALL ] && api_name_filter="$narrowed"
			fi
		fi
	fi
	for i in $(seq 1 "$api_shards"); do
		save=false
		[ "$i" -eq 1 ] && save=true
		lane "api-$i" "$api_pkg" "$i/$api_shards" "-short -timeout=10m" api "$save" "$api_name_filter"
		api_checks+=("Test (api-$i)")
	done
fi

# test/e2e gets its own lane (and test/e2e/reconcile too on full live runs)
# so the heaviest one sets the floor alone. Full live runs also isolate the template fleet test (about 1,000 s
# sequentially, bounded-parallel inside the test) from the rest of test/e2e.
e2e_pkg="github.com/GLINCKER/levelrail/test/e2e"
reconcile_pkg="github.com/GLINCKER/levelrail/test/e2e/reconcile"
fleet_test="TestTemplateFleet_Live_DeploysAndTearsDownCleanly"
docker_rest=()
for pkg in "${docker[@]}"; do
	case "$pkg" in
	"$e2e_pkg")
		if [ "$live" = full ]; then
			lane e2e-fleet "$pkg" "1/1" "-short -timeout=27m" docker false "$fleet_test"
			rest_checks+=("Test (e2e-fleet)")
			lane e2e "$pkg" "" "-short -timeout=27m" docker false "" "$fleet_test"
		else
			lane e2e "$pkg" "" "-short -timeout=20m" docker false
		fi
		rest_checks+=("Test (e2e)")
		;;
	"$reconcile_pkg")
		if [ "$live" = full ]; then
			lane e2e-reconcile "$pkg" "" "-short -timeout=27m" docker false
			rest_checks+=("Test (e2e-reconcile)")
		else
			docker_rest+=("$pkg")
		fi
		;;
	*) docker_rest+=("$pkg") ;;
	esac
done

other=$((${#docker_rest[@]} + ${#rest[@]}))
# Remaining Docker-backed packages run real container lifecycles, not unit
# logic; timeout-minutes: 32 on the job already budgets for this.
if [ "$scope" != all ] && [ "$other" -gt 0 ] && [ "$other" -le "$small_max" ]; then
	lane rest "$(join "${docker_rest[@]}" "${rest[@]}")" "" "-short -p 2 -timeout=27m" rest false
	rest_checks+=("Test (rest)")
else
	if [ "${#docker_rest[@]}" -gt 0 ]; then
		lane docker "$(join "${docker_rest[@]}")" "" "-short -p 2 -timeout=27m" docker true
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
echo "live=$live"
echo "live_reason=$live_reason"

{
	echo "go scope: $scope (${#affected[@]} affected package(s))"
	echo "lanes: ${lanes[*]:-none} (api ${#api[@]}, docker ${#docker[@]}, rest ${#rest[@]})"
	echo "coverage gate: $coverage, quarantine lane: $quarantine"
	echo "live suites: $live ($live_reason)"
} >&2
