#!/usr/bin/env bash
# Runs gen-agent-proto.sh inside a pinned image so protoc is identical everywhere.
# The image must be pinned by digest in LEVELRAIL_PROTO_IMAGE; a floating tag is refused.
set -euo pipefail
root="$(cd "$(dirname "$0")/.." && pwd)"
image="${LEVELRAIL_PROTO_IMAGE:-}"
case "$image" in
  *@sha256:*) ;;
  *) echo "set LEVELRAIL_PROTO_IMAGE to an image pinned by digest (name@sha256:...)" >&2; exit 1 ;;
esac
docker run --rm -v "$root:/work" -w /work "$image" \
  sh -c 'scripts/proto-tools.sh && scripts/gen-agent-proto.sh'
