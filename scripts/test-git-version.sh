#!/usr/bin/env bash
# Asserts git-version.sh ignores module tags (kit/v*) and picks the newest v* tag.
set -euo pipefail
script="$(cd "$(dirname "$0")" && pwd)/git-version.sh"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT
cd "$tmp"
git init -q .
git config user.email t@example.invalid
git config user.name t
git commit -q --allow-empty -m one
git tag v1.2.3
git commit -q --allow-empty -m two
git tag kit/v0.1.0
got="$("$script" "$tmp")"
case "$got" in
	v1.2.3-1-g*) ;;
	*) echo "FAIL: expected v1.2.3-1-g<sha>, got $got" >&2; exit 1 ;;
esac
echo "ok: $got"
