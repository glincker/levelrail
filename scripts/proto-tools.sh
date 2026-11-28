#!/usr/bin/env bash
# Installs the protobuf plugins at the versions named in the generated headers
# into a repo-local bin dir so regeneration is reproducible across machines.
# protoc itself comes from PATH (brew install protobuf) or scripts/gen-proto-docker.sh.
set -euo pipefail
root="$(cd "$(dirname "$0")/.." && pwd)"
bin="${LEVELRAIL_TOOLS_BIN:-$root/.tools/bin}"
mkdir -p "$bin"
gen_go="$(sed -n 's#^// 	protoc-gen-go \(v[0-9.]*\)$#\1#p' "$root/internal/agent/agentpb/agent.pb.go" | head -1)"
gen_grpc="$(sed -n 's#^// - protoc-gen-go-grpc \(v[0-9.]*\)$#\1#p' "$root/internal/agent/agentpb/agent_grpc.pb.go" | head -1)"
[ -n "$gen_go" ] && [ -n "$gen_grpc" ] || { echo "cannot read plugin versions from headers" >&2; exit 1; }
GOBIN="$bin" go install "google.golang.org/protobuf/cmd/protoc-gen-go@$gen_go"
GOBIN="$bin" go install "google.golang.org/grpc/cmd/protoc-gen-go-grpc@$gen_grpc"
echo "installed protoc-gen-go $gen_go and protoc-gen-go-grpc $gen_grpc into $bin"
