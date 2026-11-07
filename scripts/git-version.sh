#!/usr/bin/env bash
# Prints the product version from git: nearest v* tag, never a module tag such as kit/v0.1.0.
# On a release/vX.Y.Z branch whose tag does not exist yet, the branch version is the base, so a
# pre-release build reads vX.Y.Z-N-gSHA instead of the previous release tag. LEVELRAIL_VERSION wins.
set -euo pipefail
cd "${1:-$(dirname "$0")/..}"

if [ -n "${LEVELRAIL_VERSION:-}" ]; then
	echo "$LEVELRAIL_VERSION"
	exit 0
fi

described="$(git describe --tags --match 'v*' --exclude 'kit/*' --always --dirty 2>/dev/null || echo dev)"
branch="$(git rev-parse --abbrev-ref HEAD 2>/dev/null || true)"
case "$branch" in
release/v[0-9]*)
	base="${branch#release/}"
	tag="$(git describe --tags --abbrev=0 --match 'v*' --exclude 'kit/*' 2>/dev/null || true)"
	if [ -n "$tag" ] && [ "$tag" != "$base" ] && [ "${described#"$base"}" = "$described" ]; then
		count="$(git rev-list --count "${tag}..HEAD" 2>/dev/null || echo 0)"
		dirty=""
		[ -n "$(git status --porcelain --untracked-files=no 2>/dev/null)" ] && dirty="-dirty"
		echo "${base}-${count}-g$(git rev-parse --short=9 HEAD)${dirty}"
		exit 0
	fi
	;;
esac
echo "$described"
