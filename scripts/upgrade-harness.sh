#!/usr/bin/env bash
# Upgrades real earlier releases to the current tree with seeded data, then
# checks restore points, automatic restore and the downgrade refusal.
#
# Usage:
#   scripts/upgrade-harness.sh                  # last four tags
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

# Version list: arguments, then $UPGRADE_HARNESS_FROM (comma list), then
# $UPGRADE_HARNESS_VERSIONS_FILE (one tag per line), else the last four released tags.
if [ "$#" -gt 0 ]; then
	tags="$(IFS=,; echo "$*")"
elif [ -n "${UPGRADE_HARNESS_FROM:-}" ]; then
	tags="$UPGRADE_HARNESS_FROM"
elif [ -n "${UPGRADE_HARNESS_VERSIONS_FILE:-}" ]; then
	tags="$(grep -E '^v[0-9]' "$UPGRADE_HARNESS_VERSIONS_FILE" | paste -sd, -)"
else
	tags="$(git tag --list 'v[0-9]*' --sort=-v:refname | grep -v -- '-' | head -"${UPGRADE_HARNESS_COUNT:-4}" | paste -sd, -)"
fi
if [ -z "$tags" ]; then
	echo "upgrade-harness: no release tags found, run git fetch --tags" >&2
	exit 1
fi

# On linux the published server binary is used when it is available, after its
# checksum is verified; elsewhere (or on any failure) the tag is built from source.
os="$(uname -s | tr '[:upper:]' '[:lower:]')"
arch="$(uname -m)"
case "$arch" in x86_64) arch=amd64 ;; aarch64) arch=arm64 ;; esac
if [ "$os" = linux ] && command -v gh >/dev/null 2>&1; then
	cache="${LEVELRAIL_UPGRADE_FROM_BINARY_DIR:-${TMPDIR:-/tmp}/levelrail-upgrade-releases}"
	export LEVELRAIL_UPGRADE_FROM_BINARY_DIR="$cache"
	for tag in ${tags//,/ }; do
		[ -x "$cache/$tag/levelrail" ] && continue
		dl="$(mktemp -d)"
		if gh release download "$tag" -R GLINCKER/levelrail -p "levelrail-linux-$arch" -p checksums.txt -D "$dl" >/dev/null 2>&1 &&
			(cd "$dl" && grep " levelrail-linux-$arch\$" checksums.txt | sha256sum -c - >/dev/null 2>&1); then
			mkdir -p "$cache/$tag"
			install -m 0755 "$dl/levelrail-linux-$arch" "$cache/$tag/levelrail"
		else
			echo "upgrade-harness: no verified download for $tag, building from source" >&2
		fi
		rm -rf "$dl"
	done
fi

echo "upgrade-harness: upgrading from $tags"
LEVELRAIL_UPGRADE_FROM_TAGS="$tags" go test ./cmd/levelrail -run 'TestReleaseUpgrade(Harness|SchemaMigration)' -count=1 -v -timeout 45m
