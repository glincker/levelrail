#!/usr/bin/env bash
# Checks that scripts/ci-shard-names.sh, applied to the real internal/api test
# list, runs every test exactly once for each shard count the lanes use.
# It compares the union of the shards with `go test -list` output.
#
# Usage: scripts/test-api-shards.sh [shard-count...]   (default: 1 2 3 4 6)
set -uo pipefail

cd "$(git rev-parse --show-toplevel)" || exit 2

counts=("$@")
[ "${#counts[@]}" -gt 0 ] || counts=(1 2 3 4 6)

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

go test -list '.*' ./internal/api 2>/dev/null | grep -E '^(Test|Example|Fuzz)' | sort >"$tmp/all" || exit 2
total="$(wc -l <"$tmp/all" | tr -d ' ')"
if [ "$total" -eq 0 ]; then
	echo "test-api-shards: go test -list returned no tests" >&2
	exit 2
fi

fail=0
for n in "${counts[@]}"; do
	: >"$tmp/union"
	i=1
	while [ "$i" -le "$n" ]; do
		scripts/ci-shard-names.sh "$i" "$n" <"$tmp/all" >>"$tmp/union"
		i=$((i + 1))
	done
	sort "$tmp/union" >"$tmp/union.sorted"
	if ! cmp -s "$tmp/union.sorted" "$tmp/all"; then
		echo "FAIL $n shards: union differs from go test -list"
		diff "$tmp/all" "$tmp/union.sorted" | head -n 20
		fail=1
	fi
done

if [ "$fail" -ne 0 ]; then
	echo "test-api-shards: FAILED"
	exit 1
fi
echo "test-api-shards: $total tests covered exactly once for shard counts: ${counts[*]}"
