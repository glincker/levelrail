#!/usr/bin/env bash
# Runs the live Gitea and Forgejo test against throwaway local containers.
# The test removes its containers; the images go too unless KEEP_IMAGES=1.
set -euo pipefail

cd "$(dirname "$0")/../.."
command -v docker >/dev/null || { echo "docker is required" >&2; exit 2; }

export LEVELRAIL_FORGE_LIVE=1
# The control plane's SSRF guard refuses loopback targets unless told otherwise.
export APP_GIT_ALLOW_PRIVATE_NETWORKS=true

status=0
go test -count=1 -timeout 15m -run TestLiveForge -v ./internal/giteaapp || status=$?

if [ "${KEEP_IMAGES:-0}" != "1" ]; then
  docker rmi -f gitea/gitea:1.22-rootless codeberg.org/forgejo/forgejo:11-rootless >/dev/null 2>&1 || true
fi
exit "$status"
