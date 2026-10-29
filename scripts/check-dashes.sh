#!/usr/bin/env bash
# Whole-tree em/en dash sweep over tracked text files, same exclusions as the
# pre-commit hook. Usage: scripts/check-dashes.sh
set -uo pipefail
cd "$(git rev-parse --show-toplevel)" || exit 2

# Built from octal so this file does not contain the characters it hunts.
pattern="$(printf '\342\200\224')|$(printf '\342\200\223')"
hits="$(git grep -I -l -E -e "$pattern" -- . \
	':(exclude)docs-local/competitor-clones' \
	':(exclude)scripts/git-hooks/pre-commit' || true)"

if [ -n "$hits" ]; then
	echo "em dash or en dash found in:"
	echo "$hits" | sed 's/^/  /'
	echo "No em dashes or en dashes anywhere. Use commas, periods, parentheses, or colons."
	exit 1
fi
echo "check-dashes: clean"
