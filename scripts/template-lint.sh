#!/usr/bin/env bash
# Self-check for one catalog template: loader validation, submission rules and,
# with --smoke, a real run under Docker.
# Usage: scripts/template-lint.sh <id> [--smoke]
set -euo pipefail
cd "$(dirname "$0")/.."

id="${1:-}"
[ -n "$id" ] || { echo "usage: scripts/template-lint.sh <id> [--smoke]" >&2; exit 2; }
[ -d "internal/catalog/manifests/$id" ] || { echo "no such template directory: internal/catalog/manifests/$id" >&2; exit 2; }

echo "validate: loader and compose guards"
GOFLAGS=-buildvcs=false go run ./scripts/validate-templates -id "$id"

if [ "${2:-}" = "--smoke" ]; then
	echo "smoke: starting $id under Docker"
	GOFLAGS=-buildvcs=false go run ./scripts/template-smoke -only "$id" -pull -settle "${SMOKE_SETTLE:-30s}"
fi
echo "template-lint: $id ok"
