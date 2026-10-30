#!/usr/bin/env bash
# Fails on literal user-facing text in JSX of files that are NEW in the index.
# Text must go through t() with a key in web/src/locales/en/*.json (docs/i18n.md).
# Usage: scripts/check-i18n.sh [file...]   (default: tsx files added in the index)
# Escape hatch for a real non-translatable literal: end the line with "// i18n-ignore".

set -uo pipefail
cd "$(git rev-parse --show-toplevel)" || exit 2

if [ $# -gt 0 ]; then
	files=("$@")
else
	files=()
	while IFS= read -r line; do files+=("$line"); done < <(git diff --cached --name-only --diff-filter=A -- 'web/src/*.tsx')
fi

text_between_tags='>[[:space:]]*[A-Z][A-Za-z0-9 ,.'"'"'!?:()-]{3,}[[:space:]]*</'
literal_attr='(title|placeholder|aria-label|alt|label|description)="[A-Za-z][A-Za-z0-9 ,.'"'"'!?:()-]{3,}"'

fail=0
for f in ${files[@]+"${files[@]}"}; do
	case "$f" in web/src/*.tsx) ;; *) continue ;; esac
	case "$f" in *.test.tsx | web/src/routeTree.gen.ts | web/src/components/ui/*) continue ;; esac
	[ -f "$f" ] || continue
	hits=$(grep -n -E "$text_between_tags|$literal_attr" "$f" | grep -v -E -e '(^|[^A-Za-z0-9_])t\(' -e 'i18n-ignore' || true)
	if [ -n "$hits" ]; then
		echo "i18n: literal JSX text in new file $f (use useTranslation + web/src/locales/en/*.json):" >&2
		echo "$hits" | head -8 >&2
		fail=1
	fi
done
exit $fail
