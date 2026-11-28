#!/usr/bin/env bash
# Self-check for one registry template: registry rules and, with --smoke, a real
# run under Docker.
# Usage: scripts/template-lint.sh <id> [--smoke]
set -euo pipefail
cd "$(dirname "$0")/.."

id="${1:-}"
[ -n "$id" ] || { echo "usage: scripts/template-lint.sh <id> [--smoke]" >&2; exit 2; }
[ -d "registry/templates/$id" ] || { echo "no such template directory: registry/templates/$id" >&2; exit 2; }

echo "validate: registry rules, compose guards"
# The validator enforces only folders named in a changed-paths list.
changed="$(mktemp)"
trap 'rm -f "$changed"' EXIT
echo "registry/templates/$id/template.yaml" > "$changed"
GOFLAGS=-buildvcs=false go run ./scripts/validate-registry -changed "$changed"

if [ "${2:-}" = "--smoke" ]; then
	echo "smoke: starting $id under Docker"
	GOFLAGS=-buildvcs=false go run ./scripts/template-smoke -only "$id" -pull -settle "${SMOKE_SETTLE:-30s}"
fi
echo "template-lint: $id ok"
