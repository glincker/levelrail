#!/usr/bin/env bash
# Prints the space separated image tags for a release; the first is the primary.
# Usage: scripts/release/image-tags.sh <release-tag> <is-main-push: true|false>
# No sha or arch tags on purpose: moving tags only, plus exact versions.
set -euo pipefail

tag="${1:?release tag}"
is_main="${2:-false}"

if [ "$is_main" = "true" ]; then
	echo "edge"
elif [[ "$tag" == *-* ]]; then
	echo "$tag beta"
elif [[ "$tag" =~ ^v([0-9]+)\.([0-9]+)\.([0-9]+)$ ]]; then
	major="${BASH_REMATCH[1]}"
	minor="${BASH_REMATCH[2]}"
	patch="${BASH_REMATCH[3]}"
	echo "$tag ${major}.${minor}.${patch} v${major}.${minor} ${major}.${minor} v${major} ${major} latest"
else
	echo "image-tags: not a release tag: $tag" >&2
	exit 1
fi
