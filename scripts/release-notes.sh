#!/usr/bin/env bash
# Prints the squash-merge message body for a release PR: one BEGIN_NESTED_COMMIT block per
# user-visible commit, so release-please keeps every feature in CHANGELOG.md and the release
# notes even though the PR lands on main as a single commit.
# Usage: scripts/release-notes.sh <version> [base-ref] [head-ref]
#   RELEASE_NOTES_TYPES overrides the commit types listed (default: feat|fix|perf|security).
set -euo pipefail
cd "$(git rev-parse --show-toplevel)"

version="${1:?usage: scripts/release-notes.sh <version> [base-ref] [head-ref]}"
base="${2:-origin/main}"
head="${3:-origin/release/v${version}}"
types="${RELEASE_NOTES_TYPES:-feat|fix|perf|security}"

echo "Squash of the release branch. Each user-visible change is listed below so the release notes keep it."
echo
git log "${base}..${head}" --no-merges --format='%s' --reverse |
	grep -E "^(${types})(\([^)]*\))?!?: " |
	sed -E 's/ \(#[0-9]+\)$//' |
	awk '!seen[$0]++' |
	while IFS= read -r line; do
		printf 'BEGIN_NESTED_COMMIT\n%s\nEND_NESTED_COMMIT\n' "$line"
	done
echo
echo "Release-As: ${version}"
