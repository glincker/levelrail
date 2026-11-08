#!/usr/bin/env bash
# Shared helpers for the proof scripts. Source this, do not run it.

PROOF_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
export PROOF_OUT="${LEVELRAIL_PROOF_OUT:-${TMPDIR:-/tmp}/levelrail-proof}"

# tmo SECONDS cmd...: hard timeout when a timeout tool exists, else runs bare.
tmo() {
  local secs="$1"
  shift
  if command -v timeout >/dev/null 2>&1; then
    timeout "$secs" "$@"
  elif command -v gtimeout >/dev/null 2>&1; then
    gtimeout "$secs" "$@"
  else
    echo "proof: no timeout tool found, running without a hard limit" >&2
    "$@"
  fi
}

# lowprio cmd...: every heavy step runs at the lowest priority.
lowprio() { nice -n 15 "$@"; }

# find_bin NAME: existing binary only, never builds.
find_bin() {
  local c
  for c in "$PROOF_ROOT/$1" "$PROOF_ROOT/bin/$1" "$PROOF_ROOT/dist/$1"; do
    [ -x "$c" ] && { echo "$c"; return 0; }
  done
  return 1
}

# changed_files: the PR's changed files, one per line.
changed_files() {
  local base="${LEVELRAIL_PROOF_BASE:-origin/main}"
  git -C "$PROOF_ROOT" diff --name-only "$base...HEAD" 2>/dev/null || true
}

# glob_hits GLOBLIST FILES: exit 0 when any file matches any comma separated glob.
glob_hits() {
  local globs="$1" files="$2" g f
  local IFS=','
  for g in $globs; do
    g="$(echo "$g" | xargs)"
    while IFS= read -r f; do
      # shellcheck disable=SC2254
      case "$f" in $g) return 0 ;; esac
    done <<<"$files"
  done
  return 1
}
