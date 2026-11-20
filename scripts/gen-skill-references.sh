#!/usr/bin/env bash
# Regenerates the skill command references from the CLI command tree.
# Generates into a temp dir first so a failed generator never deletes the committed tree.
set -euo pipefail
cd "$(git rev-parse --show-toplevel)"
dest=skills/levelrail/references/commands
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT
if ! go run ./cmd/levelrail-cli docs --out-dir "$tmp/out"; then
  echo "gen-skill-references: generator failed, $dest left untouched" >&2
  exit 1
fi
if [ -z "$(ls -A "$tmp/out" 2>/dev/null)" ]; then
  echo "gen-skill-references: generator produced no files, $dest left untouched" >&2
  exit 1
fi
rm -rf "$dest"
mkdir -p "$(dirname "$dest")"
mv "$tmp/out" "$dest"
