#!/usr/bin/env bash
# Dumps the seeded pre-upgrade control plane databases the matrix test builds, for inspection.
set -euo pipefail
cd "$(dirname "$0")/../.."
out="${1:-$(mktemp -d)}"
LEVELRAIL_UPGRADE_FIXTURE_DIR="$out" CGO_ENABLED=0 go test ./cmd/levelrail -run TestUpgradeMatrixForwardMigration -count=1
echo "fixtures written to $out"
