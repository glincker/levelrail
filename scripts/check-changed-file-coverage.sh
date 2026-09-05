#!/usr/bin/env bash
# Changed-line coverage gate: the statements a PR actually added or
# modified under internal/ must themselves meet the threshold, not just
# the repo-wide average scripts/check-coverage.sh enforces.
#
# A whole-file version of this check was tried first and rejected: this
# codebase has plenty of individually-sub-70% files that the aggregate
# internal/ average (currently >=70%) comfortably absorbs, so gating on
# "does every file a PR touches happen to average 70%" would fail on a
# one-line change to an old, never-fully-tested file for reasons that
# have nothing to do with the PR's own new code. That's the false
# positive scripts/check-coverage.sh's own aggregate gate was already
# correctly avoiding, and it would make this gate noisy enough to get
# ignored or disabled, worse than not having it. Real per-line diff
# coverage is what CLAUDE.md's own "new code cannot lower the bar"
# framing actually asks for, so that's what this computes: only the
# `go tool cover` statement blocks that overlap a line the diff added
# or changed count toward the percentage.
#
# Usage: scripts/check-changed-file-coverage.sh <profile> <base-ref> <path-prefix> <threshold-percent>
# Example: scripts/check-changed-file-coverage.sh coverage.out origin/main internal/ 70
#
# base-ref must already be fetched (a plain ref/SHA, not a remote name
# to fetch); the caller is responsible for `git fetch` before invoking
# this script, the same read-only-with-respect-to-git-state boundary
# scripts/check-coverage.sh already keeps.

set -euo pipefail

PROFILE="${1:?usage: check-changed-file-coverage.sh <profile> <base-ref> <path-prefix> <threshold>}"
BASE_REF="${2:?usage: check-changed-file-coverage.sh <profile> <base-ref> <path-prefix> <threshold>}"
PREFIX="${3:?usage: check-changed-file-coverage.sh <profile> <base-ref> <path-prefix> <threshold>}"
THRESHOLD="${4:?usage: check-changed-file-coverage.sh <profile> <base-ref> <path-prefix> <threshold>}"

if [ ! -f "$PROFILE" ]; then
  echo "coverage profile not found: $PROFILE" >&2
  exit 1
fi

# --diff-filter=ACMR: added/copied/modified/renamed files a PR actually
# changed the content of. A deleted file has nothing left to gate.
mapfile -t CHANGED_FILES < <(
  git diff --name-only --diff-filter=ACMR "${BASE_REF}...HEAD" -- "${PREFIX}*.go" |
    grep -v '_test\.go$' |
    grep -v '\.pb\.go$' |
    grep -v '_grpc\.pb\.go$' || true
)

if [ "${#CHANGED_FILES[@]}" -eq 0 ]; then
  echo "no changed files under '${PREFIX}' between ${BASE_REF} and HEAD, nothing to gate"
  exit 0
fi

MODULE="$(head -n1 go.mod | awk '{print $2}')"
TOTAL_STMTS=0
COVERED_STMTS=0
REPORT="$(mktemp)"
trap 'rm -f "$REPORT"' EXIT

for FILE in "${CHANGED_FILES[@]}"; do
  if [ ! -f "$FILE" ]; then
    continue
  fi

  # -U0: zero context lines, so every hunk's "+a,b" is exactly the
  # lines the PR added or changed in the new file, nothing surrounding
  # them. A hunk with no "+" range at all (a pure deletion) contributes
  # nothing here, correctly: deleted code needs no new coverage.
  mapfile -t RANGES < <(
    git diff -U0 --diff-filter=ACMR "${BASE_REF}...HEAD" -- "$FILE" |
      grep -oE '^@@ -[0-9]+(,[0-9]+)? \+[0-9]+(,[0-9]+)? @@' |
      sed -E 's/^@@ -[0-9]+(,[0-9]+)? \+([0-9]+)(,([0-9]+))? @@/\2 \4/' |
      awk '{ start=$1; len=($2=="" ? 1 : $2); if (len > 0) print start, start+len-1 }'
  )
  if [ "${#RANGES[@]}" -eq 0 ]; then
    continue
  fi

  FILE_STMTS=0
  FILE_COVERED=0
  # Strip the "<module>/<file>:" prefix before splitting, since the
  # module path itself (github.com/...) contains dots that would
  # otherwise collide with a positional split on ".": once stripped,
  # the remainder is always "startLine.startCol,endLine.endCol numStmt
  # count", unambiguous to split on "." and ",".
  while IFS= read -r BLOCK_LINE; do
    [ -z "$BLOCK_LINE" ] && continue
    RANGE_PART="$(echo "$BLOCK_LINE" | awk '{print $1}' | awk -F',' '{print $1"|"$2}')"
    START_LINE="${RANGE_PART%%.*}"
    END_LINE="${RANGE_PART#*|}"
    END_LINE="${END_LINE%%.*}"
    NUM_STMT="$(echo "$BLOCK_LINE" | awk '{print $2}')"
    COUNT="$(echo "$BLOCK_LINE" | awk '{print $3}')"

    for RANGE in "${RANGES[@]}"; do
      R_START="${RANGE% *}"
      R_END="${RANGE#* }"
      # Overlap test: NOT (block ends before the range starts, OR
      # block starts after the range ends).
      if [ "$END_LINE" -ge "$R_START" ] && [ "$START_LINE" -le "$R_END" ]; then
        FILE_STMTS=$((FILE_STMTS + NUM_STMT))
        if [ "$COUNT" -gt 0 ]; then
          FILE_COVERED=$((FILE_COVERED + NUM_STMT))
        fi
        break
      fi
    done
  done < <(grep "${MODULE}/${FILE}:" "$PROFILE" | sed "s#^.*${MODULE}/${FILE}:##" || true)

  if [ "$FILE_STMTS" -gt 0 ]; then
    FILE_PERCENT="$(awk -v c="$FILE_COVERED" -v t="$FILE_STMTS" 'BEGIN { printf "%.1f", (c/t)*100 }')"
    echo "  ${FILE}: ${FILE_COVERED}/${FILE_STMTS} touched statements covered (${FILE_PERCENT}%)" >>"$REPORT"
    TOTAL_STMTS=$((TOTAL_STMTS + FILE_STMTS))
    COVERED_STMTS=$((COVERED_STMTS + FILE_COVERED))
  fi
done

echo "Changed-line coverage under '${PREFIX}' (${BASE_REF}...HEAD):"
cat "$REPORT"

if [ "$TOTAL_STMTS" -eq 0 ]; then
  echo "no touched statements found in changed files under '${PREFIX}' (docs/comments/type-only changes), nothing to gate"
  exit 0
fi

PERCENT="$(awk -v c="$COVERED_STMTS" -v t="$TOTAL_STMTS" 'BEGIN { printf "%.1f", (c/t)*100 }')"
PERCENT_X10="$(awk -v p="$PERCENT" 'BEGIN { printf "%d", p * 10 }')"
THRESHOLD_X10="$(awk -v t="$THRESHOLD" 'BEGIN { printf "%d", t * 10 }')"

echo "Total: ${COVERED_STMTS}/${TOTAL_STMTS} touched statements covered (${PERCENT}%, threshold ${THRESHOLD}%)"

if [ "$PERCENT_X10" -lt "$THRESHOLD_X10" ]; then
  echo "FAIL: changed-line coverage ${PERCENT}% is below the ${THRESHOLD}% gate" >&2
  exit 1
fi

echo "PASS: changed-line coverage ${PERCENT}% meets the ${THRESHOLD}% gate"
