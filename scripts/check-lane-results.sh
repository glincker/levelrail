#!/usr/bin/env bash
# Passes only if every named job in this workflow run concluded "success".
# Reads the latest attempt of each job, so "re-run failed jobs" counts.
#
# Usage: scripts/check-lane-results.sh "<job name>"...
set -euo pipefail

results="$(gh api --paginate "repos/${GITHUB_REPOSITORY:?}/actions/runs/${GITHUB_RUN_ID:?}/jobs?filter=latest&per_page=100" \
	--jq '.jobs[] | "\(.name)\t\(.conclusion)"')"

fail=0
for name in "$@"; do
	conclusion="$(awk -F'\t' -v n="$name" '$1 == n { print $2 }' <<<"$results" | tail -n1)"
	echo "$name: ${conclusion:-missing}"
	[ "$conclusion" = "success" ] || fail=1
done
exit "$fail"
