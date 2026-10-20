#!/usr/bin/env bash
# Merges set-mode Go coverage profiles into one, marking a block covered if
# any input covered it, so overlapping inputs (shards, reruns) never
# double-count statements.
#
# Usage: scripts/merge-coverprofiles.sh <out> <profile>...
set -euo pipefail

out="${1:?usage: merge-coverprofiles.sh <out> <profile>...}"
shift
[ "$#" -gt 0 ] || {
	echo "merge-coverprofiles.sh: no input profiles" >&2
	exit 2
}

for f in "$@"; do
	mode="$(head -n1 "$f")"
	if [ "$mode" != "mode: set" ]; then
		echo "merge-coverprofiles.sh: $f has '$mode', want 'mode: set'" >&2
		exit 1
	fi
done

{
	echo "mode: set"
	awk 'FNR > 1 { key = $1 " " $2; if (!(key in hit) || $3 > 0) hit[key] = ($3 > 0) } END { for (k in hit) print k, hit[k] }' "$@" | sort
} >"$out"
