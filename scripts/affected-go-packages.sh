#!/usr/bin/env bash
# Computes the set of this module's own Go packages "affected" by the
# diff between base-ref and HEAD: every package with a changed file,
# plus every package that (transitively, including test-only imports)
# depends on one of those. The same shape Nx's own `affected` graph
# gives JS/TS monorepos, and what DigitalOcean's gta / jharlap's
# affected give plain Go ones, hand-rolled here in ~40 lines instead of
# adding a new external tool dependency, matching this repo's existing
# scripts/check-*.sh conventions.
#
# Lets pre-push and CI test only what a change could plausibly break
# instead of the whole module on every push. nightly.yml's full,
# no-short `go test ./...` stays the exhaustive safety net for whatever
# this under-selects, so scoping down here is a real, low-risk win, not
# a coverage regression.
#
# Usage: scripts/affected-go-packages.sh [base-ref]
# Output: one import path per line on stdout, or the single line "ALL"
# if the diff touched something the import graph can't safely scope
# down (go.mod/go.sum, a migration file every store test may read at
# runtime, or this script itself): callers should treat "ALL" as
# "fall back to ./...". Prints nothing (not even "ALL") if no Go-related
# file changed at all: nothing to test, not everything.

set -euo pipefail

cd "$(git rev-parse --show-toplevel)"

BASE_REF="${1:-origin/main}"

# Files whose content the import graph doesn't model: a change to any
# of these can affect behavior no Go package literally imports.
UNSCOPABLE_PATTERN='^(go\.mod|go\.sum|internal/store/migrations/.*\.sql|scripts/affected-go-packages\.sh)$'

mapfile -t CHANGED_FILES < <(
	git diff --name-only --diff-filter=ACDMR "${BASE_REF}...HEAD" -- '*.go' go.mod go.sum 'internal/store/migrations/*.sql' scripts/affected-go-packages.sh 2>/dev/null || true
)

if [ "${#CHANGED_FILES[@]}" -eq 0 ]; then
	exit 0
fi

for f in "${CHANGED_FILES[@]}"; do
	if [[ "$f" =~ $UNSCOPABLE_PATTERN ]]; then
		echo "ALL"
		exit 0
	fi
done

# Changed packages: the unique directories holding a changed .go file
# that still exist (a deleted file's directory may be gone entirely).
declare -A CHANGED_PKG_DIRS=()
for f in "${CHANGED_FILES[@]}"; do
	case "$f" in
	*.go)
		dir="$(dirname "$f")"
		[ -d "$dir" ] && CHANGED_PKG_DIRS["./$dir"]=1
		;;
	esac
done

if [ "${#CHANGED_PKG_DIRS[@]}" -eq 0 ]; then
	exit 0
fi

mapfile -t CHANGED_IMPORT_PATHS < <(go list "${!CHANGED_PKG_DIRS[@]}" 2>/dev/null)

if [ "${#CHANGED_IMPORT_PATHS[@]}" -eq 0 ]; then
	exit 0
fi

CHANGED_JSON="$(printf '%s\n' "${CHANGED_IMPORT_PATHS[@]}" | jq -R . | jq -s .)"

# One `go list -json ./...` pass gives every first-party package's own
# Deps/TestImports/XTestImports; walking that in memory (jq -s slurps
# the whole concatenated-JSON stream into one array) is far cheaper
# than re-invoking `go list` per candidate package.
go list -json ./... | jq -r --argjson changed "$CHANGED_JSON" -s '
  def arr: if type == "array" then . else [] end;
  map(select(
    ([.ImportPath] | inside($changed))
    or (((.Deps | arr) + (.TestImports | arr) + (.XTestImports | arr)) as $all
        | any($all[]; . as $x | $changed | index($x) != null))
  ) | .ImportPath) | sort | .[]
'
