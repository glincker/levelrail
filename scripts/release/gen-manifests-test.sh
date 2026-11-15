#!/usr/bin/env bash
# Dry-run check for the release manifest generators, using fake checksums.
# Usage: scripts/release/gen-manifests-test.sh
set -euo pipefail

here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT
tag="v9.9.9"

fail() { echo "FAIL: $*" >&2; exit 1; }

i=0
for p in linux-amd64 linux-arm64 darwin-amd64 darwin-arm64 windows-amd64.exe windows-arm64.exe; do
	i=$((i + 1))
	printf '%064d  levelrail-cli-%s\n' "$i" "$p" >>"$work/checksums.txt"
done

echo "== windows manifests"
"$here/gen-windows-manifests.sh" "$tag" "$work/checksums.txt" "$work/win"
w="$work/win/winget/manifests/g/Glincker/LevelrailCli/9.9.9"
[ -f "$w/Glincker.LevelrailCli.installer.yaml" ] || fail "winget installer manifest missing"
grep -q "download/$tag/levelrail-cli-windows-amd64.exe" "$w/Glincker.LevelrailCli.installer.yaml" || fail "winget url"
grep -q "InstallerSha256: 0*5$" "$w/Glincker.LevelrailCli.installer.yaml" || fail "winget amd64 sha"
grep -q '"version": "9.9.9"' "$work/win/scoop/levelrail-cli.json" || fail "scoop version"
if grep -rq '@[A-Z_]*@' "$work/win"; then fail "unreplaced placeholder"; fi
if "$here/gen-windows-manifests.sh" "$tag" /dev/null "$work/none" >/dev/null 2>&1; then fail "missing checksum should fail"; fi

echo "OK"
