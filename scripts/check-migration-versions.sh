#!/usr/bin/env bash
# Fails fast on two internal/store/migrations/*.sql files sharing the same
# leading NNNN version prefix. Pure shell, no Go toolchain, so it's the
# cheap early gate for a collision that would otherwise only surface once
# checkNoDuplicateVersions runs inside a full test/build.
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
