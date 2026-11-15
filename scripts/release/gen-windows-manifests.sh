#!/usr/bin/env bash
# Render the winget manifest set and the Scoop bucket JSON for a release tag
# from its checksums.txt. The Windows assets are raw .exe files, so winget
# installs them as portable and Scoop renames the download on install.
# Usage: scripts/release/gen-windows-manifests.sh <tag> <checksums.txt> <outdir>
set -euo pipefail

tag="${1:?usage: gen-windows-manifests.sh <tag> <checksums.txt> <outdir>}"
sums="${2:?usage: gen-windows-manifests.sh <tag> <checksums.txt> <outdir>}"
out="${3:?usage: gen-windows-manifests.sh <tag> <checksums.txt> <outdir>}"
version="${tag#v}"
base="https://github.com/glincker/levelrail/releases/download/${tag}"
id="Glincker.LevelrailCli"

sha_for() {
	local sum
	sum="$(awk -v f="levelrail-cli-$1" '$2 == f { print $1 }' "$sums")"
	[ -n "$sum" ] || { echo "no checksum for levelrail-cli-$1 in $sums" >&2; exit 1; }
	echo "$sum"
}

amd64="$(sha_for windows-amd64.exe)" || exit 1
arm64="$(sha_for windows-arm64.exe)" || exit 1

# Winget wants the checksum upper case.
amd64_up="$(echo "$amd64" | tr '[:lower:]' '[:upper:]')"
arm64_up="$(echo "$arm64" | tr '[:lower:]' '[:upper:]')"

wdir="$out/winget/manifests/g/Glincker/LevelrailCli/${version}"
mkdir -p "$wdir" "$out/scoop"

cat >"$wdir/${id}.yaml" <<YAML
PackageIdentifier: ${id}
PackageVersion: ${version}
DefaultLocale: en-US
ManifestType: version
ManifestVersion: 1.6.0
YAML

cat >"$wdir/${id}.installer.yaml" <<YAML
PackageIdentifier: ${id}
PackageVersion: ${version}
InstallerType: portable
Commands:
  - levelrail-cli
Installers:
  - Architecture: x64
    InstallerUrl: ${base}/levelrail-cli-windows-amd64.exe
    InstallerSha256: ${amd64_up}
  - Architecture: arm64
    InstallerUrl: ${base}/levelrail-cli-windows-arm64.exe
    InstallerSha256: ${arm64_up}
ManifestType: installer
ManifestVersion: 1.6.0
YAML

cat >"$wdir/${id}.locale.en-US.yaml" <<YAML
PackageIdentifier: ${id}
PackageVersion: ${version}
PackageLocale: en-US
Publisher: Glincker
PublisherUrl: https://github.com/glincker
PackageName: Levelrail CLI
PackageUrl: https://levelrail.com
License: Apache-2.0
LicenseUrl: https://github.com/glincker/levelrail/blob/main/LICENSE
ShortDescription: Command line client for Levelrail, a self-hosted deployment platform
Tags:
  - cli
  - deployment
  - self-hosted
ManifestType: defaultLocale
ManifestVersion: 1.6.0
YAML

cat >"$out/scoop/levelrail-cli.json" <<JSON
{
    "version": "${version}",
    "description": "Command line client for Levelrail, a self-hosted deployment platform",
    "homepage": "https://levelrail.com",
    "license": "Apache-2.0",
    "architecture": {
        "64bit": {
            "url": "${base}/levelrail-cli-windows-amd64.exe#/levelrail-cli.exe",
            "hash": "${amd64}"
        },
        "arm64": {
            "url": "${base}/levelrail-cli-windows-arm64.exe#/levelrail-cli.exe",
            "hash": "${arm64}"
        }
    },
    "bin": "levelrail-cli.exe",
    "checkver": {
        "github": "https://github.com/glincker/levelrail"
    },
    "autoupdate": {
        "architecture": {
            "64bit": {
                "url": "https://github.com/glincker/levelrail/releases/download/v\$version/levelrail-cli-windows-amd64.exe#/levelrail-cli.exe"
            },
            "arm64": {
                "url": "https://github.com/glincker/levelrail/releases/download/v\$version/levelrail-cli-windows-arm64.exe#/levelrail-cli.exe"
            }
        }
    }
}
JSON
