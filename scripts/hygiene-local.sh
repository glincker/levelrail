#!/usr/bin/env bash
# Lists the Go build cache and agent worktrees by age, and prunes only what is
# older than the cutoff. Dry run by default; --apply trims the cache. Worktrees
# are only listed, remove them with scripts/clean-agent-worktrees.sh.
#
# Usage: scripts/hygiene-local.sh [--apply]   Env: HYGIENE_MAX_AGE_DAYS (default 7)
set -uo pipefail

apply=0
[ "${1:-}" = "--apply" ] && apply=1
days="${HYGIENE_MAX_AGE_DAYS:-7}"

cache="$(go env GOCACHE 2>/dev/null)"
if [ -n "$cache" ] && [ -d "$cache" ]; then
	old="$(find "$cache" -type f -mtime +"$days" 2>/dev/null | wc -l | tr -d ' ')"
	echo "go cache: $cache ($(du -sh "$cache" 2>/dev/null | cut -f1)), $old files older than ${days}d"
	if [ "$apply" = 1 ] && [ "$old" -gt 0 ]; then
		find "$cache" -type f -mtime +"$days" -delete 2>/dev/null
		echo "go cache: pruned $old files"
	fi
fi

echo "worktrees (newest activity first):"
git worktree list --porcelain 2>/dev/null | awk '/^worktree /{print $2}' | while read -r wt; do
	[ -d "$wt" ] || continue
	age="$(git -C "$wt" log -1 --format=%cr 2>/dev/null || echo unknown)"
	dirty=""
	[ -n "$(git -C "$wt" status --porcelain 2>/dev/null)" ] && dirty=" dirty"
	echo "  $wt  last commit $age$dirty"
done
