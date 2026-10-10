#!/usr/bin/env bash
# Builds the control plane for a Linux server from this checkout, with the web
# dashboard embedded and the version stamped from git, so a hand swap shows
# the right version on the Updates page.
#
# Usage: scripts/build-linux.sh [amd64|arm64] [output]   (default: amd64, ./dist/levelrail)
set -euo pipefail
cd "$(dirname "$0")/.."

arch="${1:-amd64}"
out="${2:-dist/levelrail}"
case "$arch" in amd64 | arm64) ;; *) echo "arch must be amd64 or arm64" >&2; exit 2 ;; esac

version="$(git describe --tags --always --dirty 2>/dev/null || echo dev)"
commit="$(git rev-parse --short HEAD)"

echo "==> web ($commit)"
(cd web && { [ -d node_modules ] || npm ci; } && npx vite build >/dev/null)

echo "==> go build linux/$arch as $version"
mkdir -p "$(dirname "$out")"
GOOS=linux GOARCH="$arch" CGO_ENABLED=0 go build -tags embedweb -trimpath \
	-ldflags "-s -w -X github.com/GLINCKER/levelrail/internal/version.Version=${version}-${commit}" \
	-o "$out" ./cmd/levelrail

echo "==> $out"
ls -lh "$out"
