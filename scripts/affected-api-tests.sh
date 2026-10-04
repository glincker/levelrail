#!/usr/bin/env bash
# Narrows internal/api's test run to the tests that reach a changed
# dependency, instead of the whole ~600-file package. Grows a fixed
# point of affected files via cross-file identifier grep, exempting
# routes.go wiring and a struct field nothing else reads (otherwise
# those two near-universal patterns swallow almost every change).
# Rationale/examples: docs-local/api-test-narrowing-findings-2026-10-04.md

# Usage: scripts/affected-api-tests.sh <changed-package-import-path>...
# Output: ALL, or a "|"-joined -run regex of Test/Example/Fuzz names.
set -euo pipefail
cd "$(git rev-parse --show-toplevel)"

api_dir="internal/api"
max_rounds=20
max_affected=40

[ "$#" -gt 0 ] || { echo ALL; exit 0; }
mapfile -t changed_pkgs < <(printf '%s\n' "$@" | sort -u)
changed_json="$(printf '%s\n' "${changed_pkgs[@]}" | jq -R . | jq -s .)"

mapfile -t api_files < <(cd "$api_dir" && ls -1 ./*.go 2>/dev/null | sed 's#^\./##')
[ "${#api_files[@]}" -gt 0 ] || { echo ALL; exit 0; }

# Same Deps-based transitivity scripts/affected-go-packages.sh already
# relies on: one global call instead of one "go list -deps" per import.
reach_json="$(go list -e -json ./... | jq -c --argjson changed "$changed_json" -s '
	def arr: if type == "array" then . else [] end;
	($changed | map({key: ., value: true}) | from_entries) as $set
	| map(select(([.ImportPath] + (.Deps | arr)) as $all | any($all[]; $set[.] == true)) | .ImportPath)
')" || { echo ALL; exit 0; }

file_imports() {
	{ sed -n '/^import (/,/^)/p' "$1"; grep -E '^import "' "$1" 2>/dev/null; } | grep -oE '"[^"]+"' | tr -d '"'
}

declare -A affected=()
for f in "${api_files[@]}"; do
	path="$api_dir/$f"
	while IFS= read -r imp; do
		[ -n "$imp" ] || continue
		if jq -e --arg p "$imp" 'index($p) != null' <<<"$reach_json" >/dev/null 2>&1; then
			affected["$path"]=1
			break
		fi
	done < <(file_imports "$path")
done
[ "${#affected[@]}" -gt 0 ] || { echo ALL; exit 0; }

# Is $1 (a field name from a composite-literal line) read anywhere in the
# package other than $2 (the file whose line it came from) and the
# current affected set? If not, the one field that line could leak
# through is never observed outside the file that just called into it.
field_contained() {
	local field="$1" init_file="$2" f path search=()
	for f in "${api_files[@]}"; do
		path="$api_dir/$f"
		[ "$path" = "$init_file" ] && continue
		[ -n "${affected[$path]+x}" ] && continue
		search+=("$path")
	done
	[ "${#search[@]}" -eq 0 ] && return 0
	! grep -qwF "$field" "${search[@]}" 2>/dev/null
}

idents_file="$(mktemp -t levelrail-api-idents.XXXXXX)"
trap 'rm -f "$idents_file"' EXIT

round=0
grew=true
while [ "$grew" = true ]; do
	round=$((round + 1))
	if [ "$round" -gt "$max_rounds" ] || [ "${#affected[@]}" -gt "$max_affected" ]; then
		echo ALL
		exit 0
	fi
	grew=false

	other_files=()
	for f in "${api_files[@]}"; do
		path="$api_dir/$f"
		[ -n "${affected[$path]+x}" ] || other_files+=("$path")
	done
	[ "${#other_files[@]}" -gt 0 ] || break

	: >"$idents_file"
	for path in "${!affected[@]}"; do
		go run ./scripts/go-top-level-idents "$path" 2>/dev/null
	done | sort -u >"$idents_file"
	[ -s "$idents_file" ] || break

	hits="$(grep -wFnHf "$idents_file" "${other_files[@]}" 2>/dev/null || true)"
	[ -n "$hits" ] || break

	while IFS=: read -r hit_file _hit_line hit_content; do
		[ -n "$hit_file" ] || continue
		[ -n "${affected[$hit_file]+x}" ] && continue # already pulled in this round
		# A real reference is bare code: strip quoted string bodies first
		# (so a "//" inside one, e.g. a doc URL, can't be mistaken for a
		# line comment), then strip the line comment.
		code="$(printf '%s' "$hit_content" | sed -E 's/"[^"]*"/""/g; s#//.*$##')"
		grep -qwFf "$idents_file" <<<"$code" || continue

		case "$hit_file" in
		"$api_dir"/routes*.go)
			if printf '%s' "$code" | grep -qF 'HandleFunc('; then
				continue
			fi
			;;
		esac

		field="$(printf '%s' "$code" | grep -oE '^[[:space:]]*[A-Za-z_][A-Za-z0-9_]*:' | sed -E 's/^[[:space:]]*//; s/:$//' || true)"
		if [ -n "$field" ] && field_contained "$field" "$hit_file"; then
			continue
		fi

		affected["$hit_file"]=1
		grew=true
	done <<<"$hits"
done

test_names="$(for path in "${!affected[@]}"; do
	case "$path" in
	*_test.go) grep -ho '^func \(Test\|Example\|Fuzz\)[A-Za-z0-9_]*' "$path" 2>/dev/null | sed 's/^func //' ;;
	esac
done | sort -u | paste -sd'|' -)"

if [ -z "$test_names" ]; then
	echo ALL
	exit 0
fi
echo "$test_names"
