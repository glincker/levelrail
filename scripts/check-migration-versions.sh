#!/usr/bin/env bash
# Fails fast on two internal/store/migrations/*.sql files sharing the same
# leading NNNN version prefix. Pure shell, no Go toolchain, so it's the
# cheap early gate for a collision that would otherwise only surface once
# checkNoDuplicateVersions runs inside a full test/build.
# --against-remotes also warns when another origin/* branch uses the same number
# under a different file name (MIGRATION_REMOTE_STRICT=1 turns it into a failure).
set -euo pipefail

cd "$(git rev-parse --show-toplevel)"

MIGRATIONS_DIR="internal/store/migrations"

if [ ! -d "$MIGRATIONS_DIR" ]; then
	echo "migrations directory not found: $MIGRATIONS_DIR" >&2
	exit 1
fi

versions_file="$(mktemp)"
trap 'rm -f "$versions_file"' EXIT

count=0
for f in "$MIGRATIONS_DIR"/*.sql; do
	[ -f "$f" ] || continue
	base="$(basename "$f")"
	version="${base%%_*}"
	case "$version" in
	'' | *[!0-9]*)
		echo "skipping non-versioned migration file: $base" >&2
		continue
		;;
	esac
	echo "$version $base" >>"$versions_file"
	count=$((count + 1))
done

duplicates="$(cut -d' ' -f1 "$versions_file" | sort | uniq -d)"

if [ -n "$duplicates" ]; then
	echo "duplicate migration version numbers found:" >&2
	for v in $duplicates; do
		grep "^$v " "$versions_file" | cut -d' ' -f2 >&2
	done
	exit 1
fi

echo "PASS: no duplicate migration version numbers ($count files checked)"

if [ "${1:-}" = "--against-remotes" ]; then
	remote_rows="$(NEXT_MIGRATION_NO_FETCH="${NEXT_MIGRATION_NO_FETCH:-0}" "$(dirname "$0")/next-migration.sh" --all)"
	conflicts=0
	for f in "$MIGRATIONS_DIR"/*.sql; do
		[ -f "$f" ] || continue
		base="$(basename "$f")"
		v="${base%%_*}"
		other="$(printf '%s\n' "$remote_rows" | awk -v v="$v" -v b="$base" '$1 == v && $2 != b { print $2 " (" $3 ")" }')"
		if [ -n "$other" ]; then
			echo "warning: $base shares number $v with: $other" >&2
			conflicts=$((conflicts + 1))
		fi
	done
	if [ "$conflicts" -gt 0 ] && [ "${MIGRATION_REMOTE_STRICT:-0}" = 1 ]; then
		exit 1
	fi
	echo "remote migration check: $conflicts conflicting numbers"
fi
