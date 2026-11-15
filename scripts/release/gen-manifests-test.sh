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

echo "== aur and nix"
"$here/gen-aur-nix.sh" "$tag" "$work/checksums.txt" "$work/an"
grep -q '^pkgver=9.9.9$' "$work/an/aur/PKGBUILD" || fail "pkgver"
grep -q "sha256sums_x86_64=('0*1')" "$work/an/aur/PKGBUILD" || fail "aur amd64 sha"
bash -n "$work/an/aur/PKGBUILD" || fail "PKGBUILD syntax"
grep -q 'version = "9.9.9"' "$work/an/nix/flake.nix" || fail "flake version"
if "$here/gen-aur-nix.sh" "$tag" /dev/null "$work/none2" >/dev/null 2>&1; then fail "missing checksum should fail"; fi

echo "== image tags"
eq() { [ "$1" = "$2" ] || fail "tags: got '$1' want '$2'"; }
eq "$("$here/image-tags.sh" v1.4.0 false)" "v1.4.0 1.4.0 v1.4 1.4 v1 1 latest"
eq "$("$here/image-tags.sh" v1.4.0-rc.1 false)" "v1.4.0-rc.1 beta"
eq "$("$here/image-tags.sh" main true)" "edge"
if "$here/image-tags.sh" nonsense false >/dev/null 2>&1; then fail "bad tag should fail"; fi

echo "== image labels"
lab="$("$here/image-labels.sh" levelrail "A description" v1.4.0 abc123 2026-01-02T03:04:05Z)"
grep -qx 'org.opencontainers.image.version=1.4.0' <<<"$lab" || fail "label version"
grep -qx 'org.opencontainers.image.revision=abc123' <<<"$lab" || fail "label revision"
grep -qx 'org.opencontainers.image.licenses=Apache-2.0' <<<"$lab" || fail "label license"

echo "OK"
