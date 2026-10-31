#!/usr/bin/env bash
# Prints the next free migration number, counting the working tree and every
# migration file on all origin/* branches, so parallel branches never collide.
# Usage: scripts/next-migration.sh [--all]   --all lists "NNNN file ref" rows
# Env:   MIGRATION_STEP (default 1)  NEXT_MIGRATION_NO_FETCH=1 skips the fetch
#        MIGRATION_REMOTE (default origin)  MIGRATIONS_DIR (default internal/store/migrations)
set -euo pipefail
cd "$(git rev-parse --show-toplevel)"

dir="${MIGRATIONS_DIR:-internal/store/migrations}"
remote="${MIGRATION_REMOTE:-origin}"
step="${MIGRATION_STEP:-1}"

# A stale remote view is how two branches pick the same number.
if [ "${NEXT_MIGRATION_NO_FETCH:-0}" != 1 ]; then
	git fetch -q "$remote" 2>/dev/null || echo "warning: fetch failed, using cached remote branches" >&2
fi

rows="$(mktemp)"
trap 'rm -f "$rows"' EXIT

emit() { # ref, file names on stdin
	while read -r name; do
		base="$(basename "$name")"
		v="${base%%_*}"
		case "$v" in '' | *[!0-9]*) continue ;; esac
		case "$base" in *.sql) ;; *) continue ;; esac
		echo "$v $base $1" >>"$rows"
	done
}

[ -d "$dir" ] && ls "$dir" | emit working-tree
for ref in $(git for-each-ref --format='%(refname:short)' "refs/remotes/$remote/"); do
	case "$ref" in */HEAD) continue ;; esac
	git ls-tree -r --name-only "$ref" -- "$dir" 2>/dev/null | emit "$ref"
done

if [ "${1:-}" = "--all" ]; then
	sort -u "$rows"
	exit 0
fi

max="$(awk '{ if ($1 + 0 > m) m = $1 + 0 } END { print m + 0 }' "$rows")"
printf '%04d\n' $((max + step))
