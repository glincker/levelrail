#!/usr/bin/env bash
# Regenerates internal/agent/agentpb from proto/agent/v1/agent.proto.
# Needs protoc on PATH; plugins come from .tools/bin (scripts/proto-tools.sh), then GOPATH/bin.
set -euo pipefail
root="$(cd "$(dirname "$0")/.." && pwd)"
out="${1:-$root}"
PATH="$root/.tools/bin:$(go env GOPATH)/bin:$PATH"
module="$(cd "$root" && go list -m)"
cd "$root"
protoc -I . \
  --go_out="$out" --go_opt=module="$module" \
  --go-grpc_out="$out" --go-grpc_opt=module="$module" \
  proto/agent/v1/agent.proto proto/agent/v1/migration.proto
