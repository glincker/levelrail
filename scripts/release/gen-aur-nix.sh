#!/usr/bin/env bash
# Render the AUR PKGBUILD (levelrail-cli-bin) and a Nix flake for the CLI
# from a release's checksums.txt. Both wrap the raw release binaries.
# Usage: scripts/release/gen-aur-nix.sh <tag> <checksums.txt> <outdir>
set -euo pipefail

tag="${1:?usage: gen-aur-nix.sh <tag> <checksums.txt> <outdir>}"
sums="${2:?usage: gen-aur-nix.sh <tag> <checksums.txt> <outdir>}"
out="${3:?usage: gen-aur-nix.sh <tag> <checksums.txt> <outdir>}"
version="${tag#v}"
base="https://github.com/glincker/levelrail/releases/download/${tag}"

sha_for() {
	local sum
	sum="$(awk -v f="levelrail-cli-$1" '$2 == f { print $1 }' "$sums")"
	[ -n "$sum" ] || { echo "no checksum for levelrail-cli-$1 in $sums" >&2; exit 1; }
	echo "$sum"
}

linux_amd64="$(sha_for linux-amd64)" || exit 1
linux_arm64="$(sha_for linux-arm64)" || exit 1
darwin_amd64="$(sha_for darwin-amd64)" || exit 1
darwin_arm64="$(sha_for darwin-arm64)" || exit 1

mkdir -p "$out/aur" "$out/nix"

# AUR versions cannot contain a hyphen, so a prerelease tag maps to an underscore.
pkgver="${version//-/_}"

cat >"$out/aur/PKGBUILD" <<PKG
# Maintainer: Glincker <support@levelrail.com>
pkgname=levelrail-cli-bin
pkgver=${pkgver}
pkgrel=1
pkgdesc='Command line client for Levelrail, a self-hosted deployment platform'
arch=('x86_64' 'aarch64')
url='https://levelrail.com'
license=('Apache-2.0')
provides=('levelrail-cli')
conflicts=('levelrail-cli')
source_x86_64=("levelrail-cli-\${pkgver}-amd64::${base}/levelrail-cli-linux-amd64")
source_aarch64=("levelrail-cli-\${pkgver}-arm64::${base}/levelrail-cli-linux-arm64")
sha256sums_x86_64=('${linux_amd64}')
sha256sums_aarch64=('${linux_arm64}')

package() {
  if [ "\$CARCH" = "aarch64" ]; then
    install -Dm755 "\$srcdir/levelrail-cli-\${pkgver}-arm64" "\$pkgdir/usr/bin/levelrail-cli"
  else
    install -Dm755 "\$srcdir/levelrail-cli-\${pkgver}-amd64" "\$pkgdir/usr/bin/levelrail-cli"
  fi
}
PKG

cat >"$out/nix/flake.nix" <<NIX
{
  description = "Levelrail CLI, the client for a self-hosted deployment platform";

  inputs.nixpkgs.url = "github:NixOS/nixpkgs/nixpkgs-unstable";

  outputs = { self, nixpkgs }:
    let
      version = "${version}";
      base = "${base}";
      assets = {
        x86_64-linux = { file = "levelrail-cli-linux-amd64"; sha256 = "${linux_amd64}"; };
        aarch64-linux = { file = "levelrail-cli-linux-arm64"; sha256 = "${linux_arm64}"; };
        x86_64-darwin = { file = "levelrail-cli-darwin-amd64"; sha256 = "${darwin_amd64}"; };
        aarch64-darwin = { file = "levelrail-cli-darwin-arm64"; sha256 = "${darwin_arm64}"; };
      };
      forAll = f: nixpkgs.lib.genAttrs (builtins.attrNames assets) f;
    in {
      packages = forAll (system:
        let
          pkgs = nixpkgs.legacyPackages.\${system};
          a = assets.\${system};
        in {
          default = pkgs.stdenvNoCC.mkDerivation {
            pname = "levelrail-cli";
            inherit version;
            src = pkgs.fetchurl {
              url = "\${base}/\${a.file}";
              inherit (a) sha256;
            };
            dontUnpack = true;
            installPhase = ''
              install -Dm755 \$src \$out/bin/levelrail-cli
            '';
            meta = {
              description = "Command line client for Levelrail";
              homepage = "https://levelrail.com";
              license = pkgs.lib.licenses.asl20;
              mainProgram = "levelrail-cli";
            };
          };
        });
    };
}
NIX
