#!/bin/sh
# Prints the test names (read from stdin) that shard <i> of <n> runs.
# Heaviest tests are placed first onto the least loaded shard, using the
# seconds in scripts/ci-test-weights.txt; unlisted tests weigh 0.28.
# Same input and weights always give the same split, so every lane agrees.
#
# Usage: scripts/ci-shard-names.sh <i> <n> [weights-file] < names
set -eu

i="${1:?usage: ci-shard-names.sh <i> <n> [weights-file]}"
n="${2:?usage: ci-shard-names.sh <i> <n> [weights-file]}"
weights="${3:-$(dirname "$0")/ci-test-weights.txt}"
[ -f "$weights" ] || weights=/dev/null

LC_ALL=C awk -v wf="$weights" '
	BEGIN {
		while ((getline line < wf) > 0) {
			if (line ~ /^#/ || line == "") continue
			split(line, a, " ")
			w[a[1]] = a[2] + 0
		}
	}
	{ printf "%s %s\n", ($0 in w) ? w[$0] : 0.28, $0 }
' | LC_ALL=C sort -k1,1nr -k2,2 | LC_ALL=C awk -v i="$i" -v n="$n" '
	BEGIN { for (s = 1; s <= n; s++) load[s] = 0 }
	{
		best = 1
		for (s = 2; s <= n; s++) if (load[s] < load[best]) best = s
		load[best] += $1
		if (best == i) print $2
	}
'
