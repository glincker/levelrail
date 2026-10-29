#!/usr/bin/env bash
# Removes linked worktrees whose work is already on a remote branch and whose
# tree is clean. Never touches the primary worktree, the one you run it from,
# a dirty tree, or a branch with commits no remote branch contains.
# Dry run by default.
#
# Usage: scripts/clean-agent-worktrees.sh [--apply]
set -uo pipefail

apply=0
[ "${1:-}" = "--apply" ] && apply=1

here="$(git rev-parse --show-toplevel)" || exit 2
primary=""
path=""
head_sha=""
removed=0
kept=0

consider() {
	local wt="$1" head="$2"
	[ -n "$wt" ] || return 0
	[ "$wt" = "$primary" ] && return 0
	[ "$wt" = "$here" ] && return 0
	if [ -n "$(git -C "$wt" status --porcelain 2>/dev/null)" ]; then
		echo "keep (dirty):    $wt"
		kept=$((kept + 1))
		return 0
	fi
	if [ -z "$(git branch -r --contains "$head" 2>/dev/null)" ]; then
		echo "keep (unpushed): $wt"
		kept=$((kept + 1))
		return 0
	fi
	if [ "$apply" -eq 1 ]; then
		git worktree unlock "$wt" 2>/dev/null || true
		if ! git worktree remove --force "$wt"; then
			echo "keep (remove failed): $wt"
			kept=$((kept + 1))
			return 0
		fi
		echo "removed:         $wt"
	else
		echo "would remove:    $wt"
	fi
	removed=$((removed + 1))
}

while IFS= read -r line; do
	case "$line" in
	"worktree "*)
		consider "$path" "$head_sha"
		path="${line#worktree }"
		head_sha=""
		if [ -z "$primary" ]; then
			primary="$path"
			path=""
		fi
		;;
	"HEAD "*) head_sha="${line#HEAD }" ;;
	esac
done < <(git worktree list --porcelain)
consider "$path" "$head_sha"

[ "$apply" -eq 1 ] && git worktree prune
mode=""
[ "$apply" -eq 1 ] || mode=" (dry run, pass --apply)"
echo "clean-agent-worktrees: $removed removable, $kept kept$mode"
