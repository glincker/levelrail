#!/usr/bin/env bash
# Fails if the product name appears as a string in kit/ source (CLAUDE.md
# section 3). The module's own import path is the one allowed form: it is
# the module name, not a brand string.
#
# Usage: scripts/check-brand-strings.sh [file...]   default: every kit/**/*.go
set -euo pipefail

cd "$(git rev-parse --show-toplevel)"

if [ "$#" -gt 0 ]; then
	files=("$@")
else
	mapfile -t files < <(find kit -name '*.go')
fi
[ "${#files[@]}" -gt 0 ] || exit 0

hits="$(grep -nIiE 'levelrail|glinr' "${files[@]}" 2>/dev/null | grep -vF 'github.com/GLINCKER/levelrail/kit' || true)"
if [ -n "$hits" ]; then
	echo "product name found in kit/ source (use no brand strings in code):" >&2
	echo "$hits" >&2
	exit 1
fi
