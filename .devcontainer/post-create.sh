#!/usr/bin/env bash
# Builds the dashboard into the Go binary, then seeds a read-only demo dataset.
# The data dir lives outside the repo so the seed never shows up in git status.
set -euo pipefail

cd "$(dirname "$0")/.."

(cd web && npm ci && npm run build)
go build -o bin/levelrail ./cmd/levelrail

export APP_DATA_DIR="${HOME}/.levelrail-demo"
mkdir -p "${APP_DATA_DIR}"
# Seeding into a directory that already holds users is a no-op, so reruns are safe.
./bin/levelrail demo seed --seed 1

echo "Seeded demo data. Start the dashboard with:"
echo "  APP_DEMO_MODE=1 APP_DATA_DIR=\${HOME}/.levelrail-demo ./bin/levelrail demo serve"
echo "then open http://127.0.0.1:8080"
