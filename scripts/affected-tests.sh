#!/usr/bin/env bash
# Runs go test only for packages that actually changed since a base ref,
# plus every package that transitively imports one of them (via a
# source, internal test, or external test import), instead of the full
# ./... suite. A change to a low-level package still exercises its real
# callers; a change to a leaf package (most dev-agent work) exercises
# only itself and whatever consumes it, not every unrelated package in
# the module.
#
# This exists because full-suite `go test ./... -race` runs were eating
# most of the wall-clock time on every dev/QA iteration in this repo,
# including on packages nobody touched. CI (.github/workflows/ci.yml)
# already runs the real full suite on every push/PR; this script is for
# local/agent iteration, not a replacement for that.
#
# Usage:
#   scripts/affected-tests.sh                  # diff against origin/main, race on
#   scripts/affected-tests.sh HEAD~3            # diff against a specific ref
#   scripts/affected-tests.sh origin/main -- -run TestFoo -v
#
# Exits 0 with a message and does nothing if no .go files changed.

set -euo pipefail

repo_root="$(git rev-parse --show-toplevel)"
cd "$repo_root"

base="${1:-origin/main}"
if [ "$#" -gt 0 ]; then
  shift
fi
extra_args=("$@")

changed_dirs="$(git diff --name-only "$base"...HEAD -- '*.go' | xargs -n1 dirname 2>/dev/null | sort -u || true)"
if [ -z "$changed_dirs" ]; then
  echo "affected-tests.sh: no changed .go files since $base, nothing to run" >&2
  exit 0
fi

changed_pkgs=()
while IFS= read -r d; do
  pkg="$(go list "./$d" 2>/dev/null)" || continue
  changed_pkgs+=("$pkg")
done <<<"$changed_dirs"

if [ "${#changed_pkgs[@]}" -eq 0 ]; then
  echo "affected-tests.sh: changed files aren't in any buildable package, nothing to run" >&2
  exit 0
fi

# One line per module package: importpath|imports|test_imports|xtest_imports
# (comma-joined within each field). Direct imports only, deliberately:
# the BFS below accumulates transitivity across rounds instead.
graph="$(go list -f '{{.ImportPath}}|{{join .Imports ","}}|{{join .TestImports ","}}|{{join .XTestImports ","}}' ./...)"

declare -A affected
for p in "${changed_pkgs[@]}"; do
  affected["$p"]=1
done

changed_this_round=1
while [ "$changed_this_round" -eq 1 ]; do
  changed_this_round=0
  while IFS='|' read -r pkg imports test_imports xtest_imports; do
    [ -n "${affected[$pkg]:-}" ] && continue
    all_imports="${imports},${test_imports},${xtest_imports}"
    IFS=',' read -ra imp_arr <<<"$all_imports"
    for imp in "${imp_arr[@]}"; do
      [ -z "$imp" ] && continue
      if [ -n "${affected[$imp]:-}" ]; then
        affected["$pkg"]=1
        changed_this_round=1
        break
      fi
    done
  done <<<"$graph"
done

pkgs=("${!affected[@]}")
mapfile -t pkgs < <(printf '%s\n' "${pkgs[@]}" | sort)

echo "affected-tests.sh: ${#changed_pkgs[@]} changed package(s), ${#pkgs[@]} affected package(s) (incl. transitive importers):" >&2
printf '  %s\n' "${pkgs[@]}" >&2

go test -race "${pkgs[@]}" "${extra_args[@]}"
