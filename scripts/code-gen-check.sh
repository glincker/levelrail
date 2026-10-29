#!/usr/bin/env bash
# Runs every checked-in generator and fails if any tracked file changes, so a
# forgotten regeneration is caught in CI. Usage: scripts/code-gen-check.sh
set -uo pipefail
cd "$(git rev-parse --show-toplevel)" || exit 2

snap() { { git status --porcelain; git diff; } | shasum | cut -d' ' -f1; }
before="$(snap)"

fail=0
run() {
	echo "code-gen-check: $*"
	"$@" >/dev/null 2>&1 || { echo "generator failed: $*" >&2; fail=1; }
}

run go run ./scripts/gen-api-reference
# Template generators feed docs/template-catalog.md, gen-template-gallery/templates.json
# and docs/self-host. CODEGEN_SKIP_TEMPLATES=1 opts out for local iteration.
if [ "${CODEGEN_SKIP_TEMPLATES:-off}" != on ]; then
	run go run ./scripts/gen-template-catalog-docs
	run go run ./scripts/gen-template-gallery
	run node scripts/gen-docs-pages.mjs
fi
# The route tree is gitignored, so this only proves it still generates.
if [ -d web/node_modules ]; then run node scripts/gen-route-tree.mjs; fi

if [ "$(snap)" != "$before" ]; then
	echo "generated files are stale, regenerate and commit:" >&2
	git status --porcelain >&2
	fail=1
fi
[ "$fail" -eq 0 ] && echo "code-gen-check: clean"
exit "$fail"
