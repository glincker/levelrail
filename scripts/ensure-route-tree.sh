#!/usr/bin/env bash
# Makes web/src/routeTree.gen.ts (gitignored) exist and match src/routes, in
# about half a second instead of a vite build. A stale tree breaks tsc -b.
set -uo pipefail
cd "$(git rev-parse --show-toplevel)" || exit 2
tree=web/src/routeTree.gen.ts
if [ -f "$tree" ] && [ -z "$(find web/src/routes -newer "$tree" -type f 2>/dev/null | head -n1)" ]; then
	exit 0
fi
[ -d web/node_modules ] || { echo "ensure-route-tree: web/node_modules missing, run npm ci in web/" >&2; exit 1; }
node scripts/gen-route-tree.mjs >/dev/null 2>&1 && touch "$tree" && [ -f "$tree" ] || { echo "ensure-route-tree: generation failed, run: node scripts/gen-route-tree.mjs" >&2; exit 1; }
