#!/usr/bin/env bash
# Builds the control plane and the node agent for a Linux server from this checkout, with the web
# dashboard embedded and the version stamped from git, so a hand swap shows
# the right version on the Updates page.
#
# Usage: scripts/build-linux.sh [amd64|arm64] [output]   (default: amd64, ./dist/levelrail)
set -euo pipefail
cd "$(dirname "$0")/.."

arch="${1:-amd64}"
out="${2:-dist/levelrail}"
case "$arch" in amd64 | arm64) ;; *) echo "arch must be amd64 or arm64" >&2; exit 2 ;; esac

version="$(bash scripts/git-version.sh)"
commit="$(git rev-parse --short HEAD)"

echo "==> web ($commit)"
(cd web && { [ -d node_modules ] || npm ci; } && npx vite build >/dev/null)

echo "==> go build linux/$arch as $version"
mkdir -p "$(dirname "$out")"
GOOS=linux GOARCH="$arch" CGO_ENABLED=0 go build -buildvcs=false -tags embedweb -trimpath \
	-ldflags "-s -w -X github.com/GLINCKER/levelrail/internal/version.Version=${version}" \
	-o "$out" ./cmd/levelrail

# The agent carries the same stamp, otherwise nodes list shows dev for a hand-built agent.
agent_out="$(dirname "$out")/levelrail-agent"
echo "==> go build agent linux/$arch as $version"
GOOS=linux GOARCH="$arch" CGO_ENABLED=0 go build -buildvcs=false -trimpath \
	-ldflags "-s -w -X github.com/GLINCKER/levelrail/internal/version.Version=${version}" \
	-o "$agent_out" ./cmd/levelrail-agent

echo "==> $out and $agent_out"
ls -lh "$out" "$agent_out"
