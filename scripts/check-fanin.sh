#!/usr/bin/env bash
# Reports how many packages transitively depend on each internal package (the
# blast radius of one edit under impact-based CI) and fails when a package's
# count exceeds its budget in scripts/fanin-budget.txt, or when an unlisted
# package passes FANIN_DEFAULT_MAX (default 20).
# Usage: scripts/check-fanin.sh [--update]   --update rewrites the budget file
set -euo pipefail
cd "$(git rev-parse --show-toplevel)"

budget=scripts/fanin-budget.txt
default_max="${FANIN_DEFAULT_MAX:-20}"
mod="$(head -n1 go.mod | awk '{print $2}')"

counts="$(go list -e -f '{{.ImportPath}} {{join .Deps " "}}' ./... | awk -v mod="$mod" '
	{ for (i = 2; i <= NF; i++) if (index($i, mod "/") == 1 && $i != $1) n[$i]++ }
	END { for (p in n) { s = p; sub(mod "/", "", s); print n[p], s } }' | sort -rn)"

if [ "${1:-}" = "--update" ]; then
	{
		echo "# Max transitive dependents per package (scripts/check-fanin.sh). Raise a number only"
		echo "# with a reason in the PR; lowering it after a split is the point."
		printf '%s\n' "$counts" | awk -v d="$default_max" '$1 > d { print $2, int($1 * 1.1) + 1 }' | sort
	} >"$budget"
	echo "wrote $budget"
	exit 0
fi

fail=0
while read -r n pkg; do
	[ -n "$pkg" ] || continue
	max="$(awk -v p="$pkg" '$1 == p { print $2 }' "$budget" 2>/dev/null || true)"
	max="${max:-$default_max}"
	if [ "$n" -gt "$max" ]; then
		echo "fan-in over budget: $pkg has $n dependents (budget $max)"
		fail=1
	fi
done <<<"$counts"

echo "top fan-in:"
printf '%s\n' "$counts" | head -8
exit "$fail"
