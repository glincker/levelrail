#!/usr/bin/env bash
# Decides which nightly lanes a diff needs, so a quiet or docs-only day runs nothing.
# Usage: scripts/nightly-scope.sh <base-sha|ALL>
# Prints GITHUB_OUTPUT-style lines: matrix (full-test include JSON), kit, install, sweep.
# NIGHTLY_LIVE_ALWAYS=true keeps the docker lane (the full live suites, which
# PRs only smoke-test) in the matrix on a day with no commits: upstream
# images drift without one.
set -euo pipefail
cd "$(git rev-parse --show-toplevel)"

base="${1:-ALL}"
all=false
if [ "$base" = "ALL" ] || ! git cat-file -e "${base}^{commit}" 2>/dev/null; then
	all=true
	pkgs="ALL"
else
	changed="$(git diff --name-only "${base}...HEAD" || true)"
	[ -n "$changed" ] || [ "${NIGHTLY_LIVE_ALWAYS:-}" = true ] || { printf 'matrix=[]\nkit=false\ninstall=false\nsweep=false\n'; exit 0; }
	pkgs=""
	[ -z "$changed" ] || pkgs="$(printf '%s\n' "$changed" | scripts/affected-go-packages.sh --stdin || true)"
	case "$changed" in
	*.github/workflows/nightly.yml* | *scripts/nightly-scope.sh* | *scripts/ci-go-test.sh*) pkgs="ALL" ;;
	esac
fi

has() { printf '%s\n' "$pkgs" | grep -qx "$1"; }
group_has() {
	[ "$pkgs" = "ALL" ] && return 0
	[ -n "$pkgs" ] || return 1
	mapfile -t list <<<"$pkgs"
	[ -n "$(scripts/go-test-groups.sh "$1" "${list[@]}")" ]
}

lanes=()
# -race is 3x or more on every lane, so the nightly applies it only where
# goroutines live (the race lane) and, weekly, on the big api and docker lanes.
flags='-shuffle=on -timeout=40m'
heavy_race=''
if [ "${NIGHTLY_RACE_ALL:-}" = true ] || [ "$(date -u +%u)" = 7 ]; then
	heavy_race='-race '
fi
if has ALL || [ "$pkgs" = "ALL" ] || has github.com/GLINCKER/levelrail/internal/api; then
	for i in 1 2 3 4; do lanes+=("{\"lane\":\"api-$i\",\"group\":\"api\",\"shard\":\"$i/4\",\"flags\":\"${heavy_race}$flags\"}"); done
fi
{ [ "${NIGHTLY_LIVE_ALWAYS:-}" = true ] || group_has docker; } && lanes+=("{\"lane\":\"docker\",\"group\":\"docker\",\"shard\":\"\",\"flags\":\"${heavy_race}-shuffle=on -p 2 -timeout=45m\"}")
group_has rest-heavy && lanes+=('{"lane":"rest-heavy","group":"rest-heavy","shard":"","flags":"-shuffle=on -timeout=40m"}')
group_has rest-light && lanes+=('{"lane":"rest-light","group":"rest-light","shard":"","flags":"-shuffle=on -timeout=40m"}')
group_has race && lanes+=('{"lane":"race","group":"race","shard":"","flags":"-race -shuffle=on -timeout=40m"}')

matrix="[$(IFS=,; echo "${lanes[*]:-}")]"
kit=false; install=false; sweep=false
if $all; then
	kit=true; install=true; sweep=true
else
	printf '%s\n' "$changed" | grep -q '^kit/' && kit=true
	printf '%s\n' "$changed" | grep -qE '^(install\.sh|scripts/.*install.*|cmd/levelrail/)' && install=true
	printf '%s\n' "$changed" | grep -qE '^(internal/(agent|backup)/|test/e2e/)' && sweep=true
	[ "$pkgs" = "ALL" ] && { kit=true; install=true; sweep=true; }
fi
printf 'matrix=%s\nkit=%s\ninstall=%s\nsweep=%s\n' "$matrix" "$kit" "$install" "$sweep"
