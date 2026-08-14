#!/usr/bin/env bash
# One canonical quality gate for both halves of this repo, so every
# agent (or human) verifies the same way instead of hand-assembling a
# different subset of checks each time. Every project agent (builder,
# QA, quality/fix) should run this before reporting done, and CI runs
# the same steps (see .github/workflows/ci.yml), just invoked directly
# there rather than through this wrapper.
#
# Usage:
#   scripts/verify.sh            # backend + frontend
#   scripts/verify.sh backend    # backend only (internal/, cmd/)
#   scripts/verify.sh frontend   # frontend only (web/)
#
# Exits non-zero on the first failing step. No em dashes or en dashes
# anywhere in this repo's engineering standards, this script included.

set -euo pipefail

repo_root="$(git rev-parse --show-toplevel)"
cd "$repo_root"

target="${1:-all}"

run_backend() {
  echo "==> go build ./..."
  go build ./...

  echo "==> go vet ./..."
  go vet ./...

  if command -v golangci-lint >/dev/null 2>&1; then
    echo "==> golangci-lint run"
    golangci-lint run
  else
    echo "==> golangci-lint not installed, skipping (CI still runs it)"
  fi

  echo "==> go test ./... -race -coverprofile=coverage.out"
  go test ./... -race -coverprofile=coverage.out

  echo "==> coverage gate (internal/, 70%)"
  scripts/check-coverage.sh coverage.out internal/ 70
}

run_frontend() {
  cd "$repo_root/web"

  echo "==> npx tsc -b"
  npx tsc -b

  echo "==> npm run lint"
  npm run lint

  echo "==> npx prettier --check ."
  npx prettier --check .

  echo "==> npm run build"
  npm run build
}

case "$target" in
  backend)
    run_backend
    ;;
  frontend)
    run_frontend
    ;;
  all)
    run_backend
    run_frontend
    ;;
  *)
    echo "usage: $0 [backend|frontend|all]" >&2
    exit 1
    ;;
esac

echo "==> verify.sh: all checks passed ($target)"
