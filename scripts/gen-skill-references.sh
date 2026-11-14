#!/usr/bin/env bash
# Regenerates the skill command references from the CLI command tree.
set -euo pipefail
cd "$(git rev-parse --show-toplevel)"
rm -rf skills/levelrail/references/commands
go run ./cmd/levelrail-cli docs --out-dir skills/levelrail/references/commands
