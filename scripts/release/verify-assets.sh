#!/usr/bin/env bash
# Fails unless the release carries the assets release.yml publishes: checksums + sigstore bundle,
# control plane linux amd64/arm64, CLI linux/darwin/windows amd64/arm64, agent binaries, deb and rpm.
set -euo pipefail

tag="${1:-}"
repo=""
[ -n "$tag" ] || { echo "usage: verify-assets.sh <tag> [--repo owner/name]" >&2; exit 2; }
shift
while [ $# -gt 0 ]; do
	case "$1" in
	--repo) repo="${2:?--repo needs a value}"; shift 2 ;;
	*) echo "unknown argument: $1" >&2; exit 2 ;;
	esac
done

expected=(checksums.txt checksums.txt.sigstore.json release-manifest.json)
for a in amd64 arm64; do
	expected+=("levelrail-linux-$a" "levelrail-agent-linux-$a" "levelrail-agent-linux-$a.deb" "levelrail-agent-linux-$a.rpm")
	for o in linux darwin; do expected+=("levelrail-cli-$o-$a"); done
	expected+=("levelrail-cli-windows-$a.exe")
done

args=(release view "$tag" --json assets --jq '.assets[].name')
[ -n "$repo" ] && args+=(--repo "$repo")
have="$(gh "${args[@]}")" || { echo "cannot read release $tag" >&2; exit 1; }

missing=()
for name in "${expected[@]}"; do
	printf '%s\n' "$have" | grep -qxF "$name" || missing+=("$name")
done

if [ ${#missing[@]} -gt 0 ]; then
	echo "release $tag is missing ${#missing[@]} asset(s):"
	printf '  %s\n' "${missing[@]}"
	exit 1
fi
echo "release $tag has all ${#expected[@]} expected assets"
