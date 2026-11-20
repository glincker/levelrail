#!/usr/bin/env bash
# Runs the tests listed in .github/flaky-tests.txt (or the packages given) N
# times locally and prints the ones that failed at least once.
# Usage: scripts/flake-local.sh [package...]   Env: FLAKE_RUNS (default 5)
set -uo pipefail
cd "$(git rev-parse --show-toplevel)" || exit 2

runs="${FLAKE_RUNS:-5}"
pkgs=("$@")
if [ "${#pkgs[@]}" -eq 0 ]; then
	# Entries look like "<package> <TestName> <issue url> <date>"; take the packages.
	while read -r pkg _; do
		[ -n "$pkg" ] && pkgs+=("$pkg")
	done < <(grep -v '^[[:space:]]*#' .github/flaky-tests.txt 2>/dev/null | sort -u)
fi
if [ "${#pkgs[@]}" -eq 0 ]; then
	echo "flake-local: no packages to run" >&2
	exit 0
fi

out="$(mktemp)"
trap 'rm -f "$out"' EXIT
for i in $(seq 1 "$runs"); do
	go test -short -count=1 "${pkgs[@]}" 2>&1 | grep -E '^--- FAIL' >>"$out" || true
done

if [ ! -s "$out" ]; then
	echo "flake-local: stable across $runs runs"
	exit 0
fi
echo "flake-local: unstable tests (failures over $runs runs)"
sed -E 's/ \([0-9.]+s\)$//' "$out" | sort | uniq -c | sort -rn
exit 1
