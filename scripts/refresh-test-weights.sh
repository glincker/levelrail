#!/bin/sh
# Rebuilds scripts/ci-test-weights.txt from gotestsum events.json files
# (the test-results-api-* artifacts of recent CI runs).
#
# Usage: scripts/refresh-test-weights.sh <events.json>... > scripts/ci-test-weights.txt
set -eu

[ "$#" -gt 0 ] || { echo "usage: refresh-test-weights.sh <events.json>..." >&2; exit 2; }
echo "# Mean wall seconds of internal/api tests that take 1s or more ($# result files)."
echo "# scripts/ci-go-test.sh packs shards greedily by these; unlisted tests weigh 0.28."
echo "# Refresh: scripts/refresh-test-weights.sh <events.json>..."
for f in "$@"; do
	jq -r 'select((.Action == "pass" or .Action == "fail") and .Test != null and (.Test | contains("/") | not)) | "\(.Test)\t\(.Elapsed)"' "$f"
done | LC_ALL=C awk -F'\t' '{ s[$1] += $2; c[$1]++ } END { for (t in s) if (s[t] / c[t] >= 1) printf "%s %.1f\n", t, s[t] / c[t] }' |
	LC_ALL=C sort -k2,2nr -k1,1
