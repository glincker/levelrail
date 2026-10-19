#!/usr/bin/env bash
# Counts literal user-facing strings (JSX text, label-like attributes) per web/src file, top N first.
# Usage: scripts/i18n-inventory.sh [N]   (default 20). Files with t( on the line are skipped.

set -uo pipefail
cd "$(git rev-parse --show-toplevel)" || exit 2
top="${1:-20}"

text_between_tags='>[[:space:]]*[A-Z][A-Za-z0-9 ,.'"'"'!?:()-]{3,}[[:space:]]*</'
literal_attr='(title|placeholder|aria-label|alt|label|description)="[A-Za-z][A-Za-z0-9 ,.'"'"'!?:()-]{3,}"'

git ls-files 'web/src/*.tsx' | grep -v -E '\.test\.tsx$|routeTree\.gen|components/ui/' | while read -r f; do
	n=$(grep -E "$text_between_tags|$literal_attr" "$f" | grep -v -E -e '(^|[^A-Za-z0-9_])t\(' -e 'i18n-ignore' | wc -l | tr -d ' ')
	[ "$n" -gt 0 ] && echo "$n $f"
done | sort -rn | head -n "$top"
