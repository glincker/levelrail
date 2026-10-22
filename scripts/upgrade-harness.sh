#!/usr/bin/env bash
# Upgrades real earlier releases to the current tree with seeded data, then
# checks restore points, automatic restore and the downgrade refusal.
#
# Usage:
#   scripts/upgrade-harness.sh                  # previous two tags
#   scripts/upgrade-harness.sh v0.8.2 v0.8.1    # named tags
#
# Each tag is built from its own source in a temporary directory (git archive
# plus go build), or taken from $LEVELRAIL_UPGRADE_FROM_BINARY_DIR/<tag>/levelrail
# when present. $LEVELRAIL_UPGRADE_TARGET_VERSION names the version the current
# build reports (default v0.8.99). Needs about 2 GiB of free disk and 3 to 5
# minutes per tag. Runs no Docker: containers are never started.

set -euo pipefail

repo_root="$(git rev-parse --show-toplevel)"
cd "$repo_root"

if [ "$#" -gt 0 ]; then
	tags="$(IFS=,; echo "$*")"
else
	tags="$(git tag --list 'v[0-9]*' --sort=-v:refname | grep -v -- '-' | head -2 | paste -sd, -)"
fi
if [ -z "$tags" ]; then
	echo "upgrade-harness: no release tags found, run git fetch --tags" >&2
	exit 1
fi

echo "upgrade-harness: upgrading from $tags"
LEVELRAIL_UPGRADE_FROM_TAGS="$tags" go test ./cmd/levelrail -run 'TestReleaseUpgradeHarness' -count=1 -v -timeout 45m
