#!/usr/bin/env bash
# Runs the read-only commands in transcripts.txt that match the diff and prints
# each as a text code block with its exact command. Output trimmed to 20 lines.
set -euo pipefail
# shellcheck source=lib.sh
. "$(dirname "${BASH_SOURCE[0]}")/lib.sh"

MAX_LINES="${LEVELRAIL_PROOF_TRANSCRIPT_LINES:-20}"
TMO="${LEVELRAIL_PROOF_TRANSCRIPT_TIMEOUT:-30}"
files="$(changed_files)"
cli="$(find_bin levelrail-cli || true)"
printed=0

while IFS='|' read -r globs cmd; do
  globs="$(echo "$globs" | xargs)"
  cmd="$(echo "$cmd" | sed -E 's/^[[:space:]]+|[[:space:]]+$//g')"
  case "$globs" in ''|'#'*) continue ;; esac
  glob_hits "$globs" "$files" || continue
  case "$cmd" in
    "levelrail-cli "*) ;;
    *) echo "proof: transcript skipped, only levelrail-cli commands are allowed: $cmd" >&2; continue ;;
  esac
  printed=1
  echo '```text'
  echo "\$ $cmd"
  if [ -z "$cli" ]; then
    echo "not built (levelrail-cli binary not found)"
  else
    args="${cmd#levelrail-cli }"
    # shellcheck disable=SC2086
    (cd "$PROOF_ROOT" && tmo "$TMO" nice -n 15 "$cli" $args 2>&1 | head -n "$MAX_LINES") || true
  fi
  echo '```'
done <"$PROOF_ROOT/scripts/proof/transcripts.txt"

[ "$printed" = 1 ] || echo "no mapped transcripts for this diff"
