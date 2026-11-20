#!/usr/bin/env bash
# Render the levelrail-agent Homebrew formula for a release tag from a release's checksums.txt.
# Usage: scripts/render-homebrew-agent-formula.sh <tag> <checksums.txt>  (formula on stdout)
set -euo pipefail

tag="${1:?usage: render-homebrew-agent-formula.sh <tag> <checksums.txt>}"
sums="${2:?usage: render-homebrew-agent-formula.sh <tag> <checksums.txt>}"
root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

sha_for() {
	local sum
	sum="$(awk -v f="levelrail-agent-$1" '$2 == f { print $1 }' "$sums")"
	[ -n "$sum" ] || { echo "no checksum for levelrail-agent-$1 in $sums" >&2; exit 1; }
	echo "$sum"
}

darwin_arm64="$(sha_for darwin-arm64)" || exit 1
darwin_amd64="$(sha_for darwin-amd64)" || exit 1

sed \
	-e "s|@TAG@|${tag}|g" \
	-e "s|@VERSION@|${tag#v}|g" \
	-e "s|@SHA_DARWIN_ARM64@|${darwin_arm64}|" \
	-e "s|@SHA_DARWIN_AMD64@|${darwin_amd64}|" \
	"$root/deploy/packaging/homebrew/levelrail-agent.rb.tmpl"
