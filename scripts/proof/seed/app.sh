#!/usr/bin/env bash
# Creates one real app record on the throwaway instance. Prints the app name.
# Needs LEVELRAIL_PROOF_URL and a levelrail-cli already logged in via HOME.
set -euo pipefail
cli="${LEVELRAIL_PROOF_CLI:?cli path required}"
name="proof-app"
"$cli" apps create "$name" --image "${LEVELRAIL_PROOF_IMAGE:-nginx:alpine}" >/dev/null 2>&1 || exit 1
echo "$name"
