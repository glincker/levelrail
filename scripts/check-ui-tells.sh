#!/bin/sh
# Ratchet for UI tells in web/src: counts per rule may not rise above the baseline.
# LEVELRAIL_UI_TELLS_MODE: ratchet (default), strict (any hit fails), off.
# LEVELRAIL_UI_TELLS_BASELINE: baseline path override. --write-baseline regenerates it.
set -eu

root=$(cd "$(dirname "$0")/.." && pwd)
cd "$root"

mode=${LEVELRAIL_UI_TELLS_MODE:-ratchet}
baseline=${LEVELRAIL_UI_TELLS_BASELINE:-scripts/ui-tells-baseline.txt}
write=0
[ "${1:-}" = "--write-baseline" ] && write=1

case "$mode" in
off)
	echo "ui-tells: skipped (mode off)"
	exit 0
	;;
ratchet | strict) ;;
*)
	echo "ui-tells: unknown LEVELRAIL_UI_TELLS_MODE '$mode'" >&2
	exit 2
	;;
esac

# Counts matching lines, minus any file path matching the exclusion regex.
scan() {
	pattern=$1
	exclude=${2:-^$}
	{ grep -rnE --include='*.tsx' --include='*.ts' --include='*.css' -e "$pattern" web/src 2>/dev/null || true; } |
		cut -d: -f1 | { grep -vE "$exclude" || true; } | wc -l | tr -d ' '
}

count_for() {
	case "$1" in
	uppercase_tracking) scan 'uppercase.*tracking-|tracking-.*uppercase' ;;
	tiny_text) scan 'text-\[(9|10|11|12)px\]' ;;
	raw_palette) scan '(text|bg|border|ring)-(red|amber|yellow|green|emerald|blue|sky|orange)-[0-9]' '^web/src/components/kit/tone\.ts$|^web/src/components/ui/badge\.tsx$' ;;
	color_literal) scan '#[0-9a-fA-F]{3,8}\b|rgb\(' '^web/src/index\.css$|^web/src/styles/[^/]*-tokens\.css$' ;;
	arbitrary_rounded) scan 'rounded-\[' ;;
	esac
}

rules="uppercase_tracking tiny_text raw_palette color_literal arbitrary_rounded"

if [ "$write" = 1 ]; then
	: >"$baseline"
	for r in $rules; do
		echo "$r $(count_for "$r")" >>"$baseline"
	done
	echo "ui-tells: baseline written to $baseline"
	exit 0
fi

status=0
for r in $rules; do
	n=$(count_for "$r")
	base=0
	if [ "$mode" = "ratchet" ]; then
		[ -f "$baseline" ] || {
			echo "ui-tells: baseline $baseline missing" >&2
			exit 2
		}
		base=$(awk -v k="$r" '$1 == k { print $2 }' "$baseline")
		base=${base:-0}
	fi
	if [ "$n" -gt "$base" ]; then
		verdict=fail
		status=1
	else
		verdict=ok
	fi
	echo "ui-tells: $r count=$n baseline=$base $verdict"
done
exit "$status"
