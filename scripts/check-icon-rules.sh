#!/usr/bin/env bash
# Icon and logo rules for the web app.
#   1. Brand, vendor and product marks come from @thesvg/react only, through
#      BrandIcon / BrandTile (named brands) or lib/templateLogos.ts (catalog).
#   2. UI icons (actions, navigation, status) come from @thesvg/ui-react, in the
#      glass look (the *GlassIcon exports). Any other icon library fails.
#      Type-only imports of UiIcon are fine (ADR 053).
#   3. No other icon library, no hand-drawn vendor SVGs, no raster logos.
# Usage: scripts/check-icon-rules.sh [file...]   (default: every web/src ts and tsx)

set -uo pipefail
cd "$(git rev-parse --show-toplevel)" || exit 2

if [ $# -gt 0 ]; then
	files=("$@")
else
	files=()
	while IFS= read -r line; do files+=("$line"); done < <(git ls-files 'web/src/*.ts' 'web/src/*.tsx')
fi

fail=0
note() { echo "icon rules: $1" >&2; fail=1; }

for f in "${files[@]}"; do
	case "$f" in web/src/*.ts | web/src/*.tsx) ;; *) continue ;; esac
	[ -f "$f" ] || continue

	if grep -n -E "from '(lucide-react|react-icons|@heroicons|@tabler/|@radix-ui/react-icons|@iconify|@fortawesome|remixicon|phosphor-react)" "$f" >/dev/null; then
		note "$f imports a second icon library; UI icons come from @thesvg/ui-react (ADR 053)"
	fi

	if grep -n -E "@phosphor-icons" "$f" >/dev/null; then
		note "$f imports another icon library; use @thesvg/ui-react glass icons (ADR 053)"
	fi

	case "$f" in
	web/src/components/BrandIcon.tsx | web/src/lib/templateLogos.ts | *.test.ts | *.test.tsx) ;;
	*)
		if grep -n -E "(from|import\()\s*'@thesvg/react" "$f" >/dev/null; then
			note "$f imports @thesvg/react directly; go through BrandIcon / BrandTile or lib/templateLogos.ts"
		fi
		;;
	esac

	if grep -n -E "<img[^>]*src=\"[^\"]*(logo|brand|vendor)[^\"]*\.(png|jpe?g|svg|webp)\"" "$f" >/dev/null; then
		note "$f renders a raster or file logo; use a @thesvg/react mark through BrandTile"
	fi
done

[ -n "${ICON_RULES_VERBOSE:-}" ] && echo "icon rules: checked ${#files[@]} files" >&2
exit "$fail"
