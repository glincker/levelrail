#!/usr/bin/env bash
# Prints this module's Go packages affected by a diff: every package with a
# changed file (including non-Go files it embeds or its tests read), plus
# every package whose build or tests transitively import one of those.
# A change only visible to a package's own tests (_test.go, testdata/, a
# file only its tests read) selects that package alone, not its dependents.
# nightly.yml's full run is the safety net for anything this under-selects.
#
# Usage: scripts/affected-go-packages.sh [base-ref]      diff base-ref...HEAD
#        scripts/affected-go-packages.sh --stdin         changed paths on stdin
#        scripts/affected-go-packages.sh --stdin --seeds changed package dirs only, no Go toolchain needed
# Output: one import path per line, or the single line "ALL" when the diff
# touched something the import graph can't scope (go.mod, go.sum, a store
# migration, this script). Nothing at all when no Go package is affected.

set -euo pipefail

cd "$(git rev-parse --show-toplevel)"

SEEDS_ONLY=false
if [ "${1:-}" = "--stdin" ]; then
	[ "${2:-}" = "--seeds" ] && SEEDS_ONLY=true
	mapfile -t CHANGED_FILES
else
	BASE_REF="${1:-origin/main}"
	mapfile -t CHANGED_FILES < <(git diff --name-only --diff-filter=ACDMR "${BASE_REF}...HEAD" 2>/dev/null || true)
fi

[ "${#CHANGED_FILES[@]}" -gt 0 ] || exit 0

# Store migrations are read at runtime by nearly every store-backed test.
UNSCOPABLE_PATTERN='^(go\.mod|go\.sum|kit/go\.mod|kit/go\.sum|internal/store/migrations/.*\.sql|scripts/affected-go-packages\.sh)$'
# Too common to map a change to the Go files that mention them by name.
GENERIC_NAMES='^(README\.md|index\.md|Dockerfile|package\.json|package-lock\.json|LICENSE|Makefile|\.gitignore|\.dockerignore)$'

for f in "${CHANGED_FILES[@]}"; do
	if [[ "$f" =~ $UNSCOPABLE_PATTERN ]]; then
		echo "ALL"
		exit 0
	fi
done

has_go() { compgen -G "$1/*.go" >/dev/null; }

module_path="$(head -n1 go.mod | awk '{print $2}')"
declare -A SEED_DIRS=() TEST_SEED_DIRS=() KIT_IMPORTS=()
add_seed() { # dir [test]
	case "$1" in
	tools | tools/*) return ;; # separate module, see ci.yml's Build, vet
	kit | kit/*)
		# Nested module: go list from the root can't resolve its dirs, but root
		# packages import it, so the import path still seeds the dependents walk.
		[ "${2:-}" = test ] || KIT_IMPORTS["$module_path/$1"]=1
		return
		;;
	esac
	[ -d "$1" ] || return 0
	if [ "${2:-}" = test ]; then
		TEST_SEED_DIRS["./$1"]=1
	else
		SEED_DIRS["./$1"]=1
	fi
}

for f in "${CHANGED_FILES[@]}"; do
	case "$f" in
	*_test.go)
		add_seed "$(dirname "$f")" test
		continue
		;;
	*.go)
		add_seed "$(dirname "$f")"
		continue
		;;
	.github/* | adr/* | web/*) continue ;;
	test/fixtures/*)
		add_seed test/e2e test
		add_seed test/e2e/reconcile test
		continue
		;;
	esac
	# A non-Go file under a package dir (embedded schema, SQL, testdata) is
	# part of that package. Markdown there is almost always prose.
	if [[ "$f" != *.md ]]; then
		dir="$(dirname "$f")"
		while [ "$dir" != "." ] && ! { [ -d "$dir" ] && has_go "$dir"; }; do
			dir="$(dirname "$dir")"
		done
		if [ "$dir" != "." ]; then
			case "$f" in
			*/testdata/*) add_seed "$dir" test ;;
			*) add_seed "$dir" ;;
			esac
			continue
		fi
	fi
	# Outside any package (docs/, root files, proto/): Go files naming it in
	# a string literal, e.g. a test that reads ../../docs/api-reference.md.
	name="$(basename "$f")"
	[[ "$name" =~ $GENERIC_NAMES ]] && continue
	if [[ "$name" =~ ^[A-Za-z0-9._-]+$ ]]; then
		grep_args=(-E -e "[\"\`][^\"\`]*${name//./\\.}[\"\`]")
	else
		grep_args=(-F -e "$name")
	fi
	while IFS= read -r hit; do
		case "$hit" in
		*_test.go) add_seed "$(dirname "$hit")" test ;;
		*) add_seed "$(dirname "$hit")" ;;
		esac
	done < <(git grep -l "${grep_args[@]}" -- '*.go' 2>/dev/null || true)
done

[ "$((${#SEED_DIRS[@]} + ${#TEST_SEED_DIRS[@]} + ${#KIT_IMPORTS[@]}))" -gt 0 ] || exit 0

if [ "$SEEDS_ONLY" = true ]; then
	printf '%s\n' "${!SEED_DIRS[@]}" "${!TEST_SEED_DIRS[@]}" | sort -u
	exit 0
fi

TEST_ONLY=()
if [ "${#TEST_SEED_DIRS[@]}" -gt 0 ]; then
	mapfile -t TEST_ONLY < <(go list -e -f '{{.ImportPath}}' "${!TEST_SEED_DIRS[@]}")
fi
if [ "${#SEED_DIRS[@]}" -eq 0 ] && [ "${#KIT_IMPORTS[@]}" -eq 0 ]; then
	printf '%s\n' "${TEST_ONLY[@]}" | sort -u
	exit 0
fi

CHANGED_IMPORT_PATHS=("${!KIT_IMPORTS[@]}")
if [ "${#SEED_DIRS[@]}" -gt 0 ]; then
	mapfile -t root_paths < <(go list -e -f '{{.ImportPath}}' "${!SEED_DIRS[@]}")
	CHANGED_IMPORT_PATHS+=("${root_paths[@]}")
fi

CHANGED_JSON="$(printf '%s\n' "${CHANGED_IMPORT_PATHS[@]}" | jq -R . | jq -s .)"

# Deps is already transitive for the package itself; test imports are
# direct only, so their own Deps are folded in to catch a test that
# imports a helper which imports the changed package.
{
	go list -e -json ./... | jq -r --argjson changed "$CHANGED_JSON" -s '
	  def arr: if type == "array" then . else [] end;
	  ($changed | map({key: ., value: true}) | from_entries) as $set
	  | (map({key: .ImportPath, value: (.Deps | arr)}) | from_entries) as $deps
	  | map(
	      ((.TestImports | arr) + (.XTestImports | arr)) as $timp
	      | ([.ImportPath] + (.Deps | arr) + $timp + ([$timp[] | $deps[.] // []] | add // [])) as $all
	      | select(any($all[]; $set[.] == true))
	      | .ImportPath)
	  | .[]
	'
	printf '%s\n' "${TEST_ONLY[@]}"
} | sed '/^$/d' | sort -u
