#!/usr/bin/env bash
# Regenerates internal/agent/agentpb from proto/agent/v1/agent.proto.
# Needs protoc, protoc-gen-go and protoc-gen-go-grpc on PATH (or in $(go env GOPATH)/bin).
set -euo pipefail
root="$(cd "$(dirname "$0")/.." && pwd)"
out="${1:-$root}"
PATH="$(go env GOPATH)/bin:$PATH"
module="$(cd "$root" && go list -m)"
cd "$root"
protoc -I . \
  --go_out="$out" --go_opt=module="$module" \
  --go-grpc_out="$out" --go-grpc_opt=module="$module" \
  proto/agent/v1/agent.proto
