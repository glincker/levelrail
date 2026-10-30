#!/bin/bash
# Keeps docs/i18n/README.<lang>.md in sync with README.md without a CI API key.
# Portable: bash 3.2 (macOS) and Linux, no mapfile, no associative arrays.
# Usage: readme-sync.sh hash | stamp <lang> | check [--strict] | validate <lang> [file] | update [lang...]
set -uo pipefail

ROOT="$(git rev-parse --show-toplevel)" || exit 1
cd "$ROOT" || exit 1
EN=README.md
DIR=docs/i18n
NAV_START='<!-- i18n-nav:start -->'
NAV_END='<!-- i18n-nav:end -->'

# The nav block is dropped so adding a language never marks every file stale.
filter_en() {
	awk -v s="$NAV_START" -v e="$NAV_END" '
		index($0, s) { skip = 1; next }
		index($0, e) { skip = 0; next }
		!skip { print }
	' "$1"
}

# Same filter for translations, plus the stamp line, so only content is compared.
filter_tr() {
	filter_en "$1" | grep -v '^<!-- translated-from:' || true
}

cur_hash() { filter_en "$EN" | git hash-object --stdin; }

tr_file() { printf '%s/README.%s.md' "$DIR" "$1"; }

stamp_of() {
	head -n 1 "$1" | sed -n 's/^<!-- translated-from: README.md blob:\([0-9a-f]*\) lang:[^ ]* -->$/\1/p'
}

cmd_hash() { cur_hash; }

cmd_stamp() {
	lang="${1:-}"
	[ -n "$lang" ] || { echo "usage: stamp <lang>" >&2; return 2; }
	f="$(tr_file "$lang")"
	[ -f "$f" ] || { echo "missing $f" >&2; return 1; }
	h="$(cur_hash)"
	tmp="$(mktemp)"
	printf '<!-- translated-from: README.md blob:%s lang:%s -->\n' "$h" "$lang" >"$tmp"
	if head -n 1 "$f" | grep -q '^<!-- translated-from:'; then
		tail -n +2 "$f" >>"$tmp"
	else
		cat "$f" >>"$tmp"
	fi
	cat "$tmp" >"$f"
	rm -f "$tmp"
	# Writing the blob keeps the old English recoverable for the next update.
	filter_en "$EN" | git hash-object -w --stdin >/dev/null
	echo "stamped $f blob:$h"
}

# Returns 0 when at least one stale file was found (used by the hook).
cmd_check() {
	strict=0
	[ "${1:-}" = "--strict" ] && strict=1
	h="$(cur_hash)"
	bad=0
	for f in "$DIR"/README.*.md; do
		[ -f "$f" ] || continue
		lang="${f#"$DIR"/README.}"
		lang="${lang%.md}"
		s="$(stamp_of "$f")"
		if [ -z "$s" ]; then
			echo "MISSING stamp $lang"
			bad=1
		elif [ "$s" != "$h" ]; then
			echo "STALE $lang"
			bad=1
		else
			echo "fresh $lang"
		fi
	done
	[ "$strict" -eq 1 ] && [ "$bad" -eq 1 ] && return 1
	return 0
}

strip_fences() {
	awk '/^[ \t]*(```|~~~)/ { f = !f; next } !f { print }'
}

ex_headings() {
	strip_fences | grep -E '^#{1,6}([ \t]|$)' | sed -E 's/^(#+).*/\1/' | awk '{ c[length($0)]++ } END { for (i = 1; i <= 6; i++) printf "h%d %d\n", i, c[i] + 0 }'
}

ex_fences() {
	awk '/^[ \t]*(```|~~~)/ { f = !f; print; next } f { print }'
}

# Fragment-only links depend on translated heading slugs, so they are skipped.
# Leading ../ is stripped because translations live two levels deeper.
ex_urls() {
	strip_fences | { grep -oE '\]\([^) ]+|(href|src|srcset)="[^"]*"' || true; } |
		sed -E 's/^\]\(//; s/^[a-z]+="//; s/"$//; s#^(\.\./)+##' |
		grep -v -E '^#|(^|/)i18n/README\.md$|^README\.md$' | sort || true
}

ex_imgs() {
	strip_fences | { grep -oE '!\[[^]]*\]\([^) ]+|src="[^"]*"' || true; } |
		sed -E 's/^!\[[^]]*\]\(//; s/^src="//; s/"$//; s#^(\.\./)+##' | sort || true
}

cmd_validate() {
	lang="${1:-}"
	[ -n "$lang" ] || { echo "usage: validate <lang> [file]" >&2; return 2; }
	f="${2:-$(tr_file "$lang")}"
	[ -f "$f" ] || { echo "missing $f" >&2; return 1; }
	w="$(mktemp -d)"
	filter_en "$EN" >"$w/en.md"
	filter_tr "$f" >"$w/tr.md"
	rc=0
	for kind in headings fences urls imgs; do
		"ex_$kind" <"$w/en.md" >"$w/en.$kind"
		"ex_$kind" <"$w/tr.md" >"$w/tr.$kind"
		if ! diff -u "$w/en.$kind" "$w/tr.$kind" >"$w/d.$kind"; then
			echo "MISMATCH $kind ($lang): '<' is English, '>' is translation"
			sed -n '3,$p' "$w/d.$kind" | grep -E '^[+-]' | sed -E 's/^-/< /; s/^\+/> /' | head -n 20
			rc=1
		fi
	done
	rm -rf "$w"
	[ "$rc" -eq 0 ] && echo "ok $lang"
	return "$rc"
}

build_prompt() {
	lang="$1"; old="$2"; out="$3"
	{
		cat <<'P'
You maintain a translation of a project README. Update the CURRENT TRANSLATION so it matches the NEW ENGLISH.
Rules:
- Change only passages whose English changed between OLD ENGLISH and NEW ENGLISH (all passages if OLD ENGLISH is empty).
- Keep every other sentence, and any human corrections, byte-for-byte as in the current translation.
- Keep code blocks, URLs and badge images identical to the NEW ENGLISH.
- Keep product terms as the glossary says.
- Keep the first-line stamp comment and the machine-translation note under the title as they are.
- Output only the full updated markdown file, with no commentary and no outer code fence.
P
		printf '\n=== GLOSSARY ===\n'
		cat "$DIR/glossary.md" 2>/dev/null
		printf '\n=== TARGET LANGUAGE ===\n%s\n' "$lang"
		printf '\n=== OLD ENGLISH ===\n'
		[ -n "$old" ] && git cat-file -p "$old" 2>/dev/null
		printf '\n=== NEW ENGLISH ===\n'
		filter_en "$EN"
		printf '\n=== CURRENT TRANSLATION ===\n'
		cat "$(tr_file "$lang")"
	} >"$out"
}

cmd_update() {
	command -v claude >/dev/null 2>&1 || { echo "claude CLI not found on PATH: install it to run update" >&2; return 2; }
	langs="$*"
	if [ -z "$langs" ]; then
		h="$(cur_hash)"
		for f in "$DIR"/README.*.md; do
			[ -f "$f" ] || continue
			l="${f#"$DIR"/README.}"; l="${l%.md}"
			[ "$(stamp_of "$f")" = "$h" ] || langs="$langs $l"
		done
	fi
	fails=0
	for lang in $langs; do
		f="$(tr_file "$lang")"
		[ -f "$f" ] || { echo "skip $lang: $f missing" >&2; fails=1; continue; }
		old="$(stamp_of "$f")"
		if [ -n "$old" ] && ! git cat-file -e "$old" 2>/dev/null; then
			echo "$lang: old English blob $old not in this repo, falling back to full retranslation"
			old=""
		fi
		w="$(mktemp -d)"
		build_prompt "$lang" "$old" "$w/prompt.txt"
		# Prompt goes via stdin and tools are off: the model only returns text.
		if ! claude -p --model sonnet --tools "" --no-session-persistence <"$w/prompt.txt" >"$w/raw.md" 2>"$w/err.txt"; then
			echo "$lang: claude failed, kept old file: $(head -n 3 "$w/err.txt")" >&2
			fails=1; rm -rf "$w"; continue
		fi
		# Drop an outer fence if the model added one.
		if head -n 1 "$w/raw.md" | grep -q '^```'; then
			sed '1d;$d' "$w/raw.md" >"$w/new.md"
		else
			cp "$w/raw.md" "$w/new.md"
		fi
		if cmd_validate "$lang" "$w/new.md" >"$w/val.txt" 2>&1; then
			cat "$w/new.md" >"$f"
			cmd_stamp "$lang"
		else
			echo "$lang: validation failed, kept old file" >&2
			cat "$w/val.txt" >&2
			fails=1
		fi
		rm -rf "$w"
	done
	return "$fails"
}

sub="${1:-}"
[ $# -gt 0 ] && shift
case "$sub" in
	hash) cmd_hash ;;
	stamp) cmd_stamp "$@" ;;
	check) cmd_check "$@" ;;
	validate) cmd_validate "$@" ;;
	update) cmd_update "$@" ;;
	*) echo "usage: $0 hash | stamp <lang> | check [--strict] | validate <lang> [file] | update [lang...]" >&2; exit 2 ;;
esac
