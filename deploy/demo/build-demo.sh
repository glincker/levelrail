#!/usr/bin/env bash
# Builds the demo binary (dashboard embedded, linux) and a tarball holding it
# with the deploy assets, ready to copy to a box and run install.sh from.
#
# Usage: deploy/demo/build-demo.sh [amd64|arm64] [out-dir]   (default: amd64, ./dist/demo)
set -euo pipefail
cd "$(dirname "$0")/../.."

arch="${1:-amd64}"
out="${2:-dist/demo}"
case "$arch" in amd64 | arm64) ;; *) echo "arch must be amd64 or arm64" >&2; exit 2 ;; esac

mkdir -p "$out"
bash scripts/build-linux.sh "$arch" "$out/levelrail"

stage="$(mktemp -d)"
trap 'rm -rf "$stage"' EXIT
bundle="$stage/levelrail-demo"
mkdir -p "$bundle"
cp "$out/levelrail" "$bundle/levelrail"
for f in deploy/demo/*; do
	[ "$(basename "$f")" = "build-demo.sh" ] || cp "$f" "$bundle/"
done

tar -C "$stage" -czf "$out/levelrail-demo-linux-$arch.tar.gz" levelrail-demo
(cd "$out" && sha256sum "levelrail-demo-linux-$arch.tar.gz" >"levelrail-demo-linux-$arch.tar.gz.sha256")
echo "==> $out/levelrail-demo-linux-$arch.tar.gz"
