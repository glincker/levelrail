#!/usr/bin/env bash
# Icon and logo rules for the web app.
#   1. Brand, vendor and product marks come from @thesvg/react only, through
#      BrandIcon / BrandTile (named brands) or lib/templateLogos.ts (catalog).
#   2. UI icons (actions, navigation, status) come from @thesvg/ui-react for all
#      new code, in the glass look (the *GlassIcon exports). The goal is to remove
#      Phosphor entirely. Existing Phosphor imports (@phosphor-icons/react/dist/ssr) stay
#      until their file is touched; add no new ones. Type-only imports of Icon are fine.
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
		note "$f imports a second icon library; UI icons are Phosphor only (@phosphor-icons/react/dist/ssr)"
	fi

	case "$f" in
	web/src/components/BrandIcon.tsx | web/src/lib/templateLogos.ts | *.test.ts | *.test.tsx) ;;
	*)
		if grep -n -E "(from|import\()\s*'@thesvg/react" "$f" >/dev/null; then
			note "$f imports @thesvg/react directly; go through BrandIcon / BrandTile or lib/templateLogos.ts"
		fi
		;;
	esac

	if grep -n -E "^import \{[^}]*\} from '@phosphor-icons/react'$" "$f" >/dev/null; then
		note "$f imports Phosphor from the package root; use @phosphor-icons/react/dist/ssr"
	fi

	if grep -n -E "<img[^>]*src=\"[^\"]*(logo|brand|vendor)[^\"]*\.(png|jpe?g|svg|webp)\"" "$f" >/dev/null; then
		note "$f renders a raster or file logo; use a @thesvg/react mark through BrandTile"
	fi
done

[ -n "${ICON_RULES_VERBOSE:-}" ] && echo "icon rules: checked ${#files[@]} files" >&2
exit "$fail"
