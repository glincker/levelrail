#!/usr/bin/env bash
# Runs web/src/i18n/checkLocale.mjs for each language whose locale files changed.
# Usage: scripts/check-locales.sh [--all] [--base <ref>]
#   default   languages touched since the merge-base with $LEVELRAIL_PUSH_BASE (else origin/main)
#   --all     every language folder under web/src/locales except en
set -uo pipefail
cd "$(git rev-parse --show-toplevel)" || exit 2

all=0
base="${LEVELRAIL_PUSH_BASE:-origin/main}"
while [ $# -gt 0 ]; do
	case "$1" in
	--all) all=1 ;;
	--base) base="$2"; shift ;;
	*) echo "usage: $0 [--all] [--base <ref>]" >&2; exit 2 ;;
	esac
	shift
done

locales=web/src/locales
if [ "$all" -eq 1 ]; then
	langs=$(find "$locales" -mindepth 1 -maxdepth 1 -type d ! -name en -exec basename {} \; | sort)
else
	mb=$(git merge-base HEAD "$base" 2>/dev/null) || mb=""
	changed=$( { [ -n "$mb" ] && git diff --name-only "$mb" -- "$locales" "web/src/i18n/missingBaseline"; git diff --name-only HEAD -- "$locales" "web/src/i18n/missingBaseline"; } 2>/dev/null | sort -u)
	langs=$(printf '%s\n' "$changed" | sed -n -e "s#^$locales/\([^/]*\)/.*#\1#p" -e 's#^web/src/i18n/missingBaseline/\(.*\)\.json$#\1#p' | grep -vx en | sort -u)
fi
[ -z "$langs" ] && exit 0
command -v node >/dev/null 2>&1 || { echo "check-locales: node missing, skipped"; exit 0; }

fail=0
for l in $langs; do
	[ -d "$locales/$l" ] || continue
	node web/src/i18n/checkLocale.mjs "$l" || fail=1
done
exit $fail
