#!/usr/bin/env bash
# Build levelrail-cli .deb and .rpm files from the linux CLI release binaries.
# Usage: scripts/release/build-cli-packages.sh <tag> <bin-dir> <out-dir>
# Set NFPM to the nfpm binary path (default: nfpm on PATH).
set -euo pipefail

tag="${1:?usage: build-cli-packages.sh <tag> <bin-dir> <out-dir>}"
bin_dir="${2:?usage: build-cli-packages.sh <tag> <bin-dir> <out-dir>}"
out_dir="${3:?usage: build-cli-packages.sh <tag> <bin-dir> <out-dir>}"
nfpm="${NFPM:-nfpm}"
root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"

mkdir -p "$out_dir"
stage="$(mktemp -d)"
trap 'rm -rf "$stage"' EXIT
mkdir -p "$stage/pkg"

# nfpm maps a prerelease like 1.2.0-beta.1 to 1.2.0~beta.1 so it sorts first.
export VERSION="${tag#v}"
for arch in amd64 arm64; do
	src="$bin_dir/levelrail-cli-linux-$arch"
	[ -f "$src" ] || { echo "missing $src" >&2; exit 1; }
	cp "$src" "$stage/pkg/levelrail-cli"
	chmod 0755 "$stage/pkg/levelrail-cli"
	export ARCH="$arch"
	for fmt in deb rpm; do
		(cd "$stage" && "$nfpm" package --config "$root/deploy/packaging/nfpm/cli.yaml" \
			--packager "$fmt" --target "$out_dir/levelrail-cli-linux-$arch.$fmt")
	done
done

(cd "$out_dir" && sha256sum levelrail-cli-linux-*.deb levelrail-cli-linux-*.rpm > package-checksums.txt)
