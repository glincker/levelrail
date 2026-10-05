#!/usr/bin/env bash
# Rehearse `auth-backfill` on a synthetic legacy database.
# Usage: scripts/auth-rehearsal/run.sh
# Env: APP_REHEARSAL_SCALE (default 10, 0 or 1 skips the scaled timing run),
#      APP_REHEARSAL_USERS / _TOKENS / _PASSKEYS / _TOTP_USERS / _CASE_DUPES / _OAUTH,
#      APP_REHEARSAL_SEED, APP_REHEARSAL_SAMPLE_USERS / _SAMPLE_TOKENS,
#      APP_REHEARSAL_DIR (keep the databases there instead of a temp dir).
set -euo pipefail
cd "$(git rev-parse --show-toplevel)"
export APP_REHEARSAL=1
export APP_REHEARSAL_SCALE="${APP_REHEARSAL_SCALE:-10}"
go test -run TestRehearsal -count=1 -v -timeout 60m ./internal/authengine 2>&1 | grep -v '^{"level"'
