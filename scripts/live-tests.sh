#!/usr/bin/env bash
# Runs the full live-Docker suites locally, the same set nightly.yml runs and
# the ci:live label runs on a PR (PRs otherwise run the smoke subset).
#
# Usage: scripts/live-tests.sh [go test args...]
#   scripts/live-tests.sh                      all of test/e2e and test/e2e/reconcile
#   scripts/live-tests.sh -run TestBreakGlass  one suite
# Environment:
#   LEVELRAIL_FLEET_PARALLEL  templates deployed at once in the fleet test (default 3)
set -euo pipefail

cd "$(git rev-parse --show-toplevel)"

unset LEVELRAIL_LIVE_SUITE
exec go test -short -count=1 -timeout=40m ./test/e2e/... "$@"
