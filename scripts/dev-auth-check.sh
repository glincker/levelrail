#!/usr/bin/env bash
#
# dev-auth-check.sh: local sanity check for the API ability boundaries
# (internal/api/router.go's requireAbility calls), using the fixed
# plaintext tokens in dev-fixtures.yml instead of registering an admin,
# logging in, and minting a token by hand every time.
#
# Assumptions:
#   - A real Levelrail control plane binary is ALREADY RUNNING locally
#     with APP_DEV_MODE=1 set (a non "-tags embedweb" build, see
#     internal/api/devmode_debug.go / devmode_release.go and
#     adr/013-backend-dev-mode-build-tag-gate.md). This script does not
#     start or stop the server, that is out of scope.
#   - dev-fixtures.yml exists at the repo root (or wherever
#     APP_DEV_FIXTURES_FILE pointed the running server at, see below)
#     and has not been edited to remove the six stock fixture tokens
#     (dev-read, dev-read-sensitive, dev-write, dev-write-sensitive,
#     dev-deploy, dev-root).
#   - No APP_MASTER_KEY was set on the running server, i.e. it is a bare
#     local dev server with no secrets manager configured. That is why
#     PUT .../secrets/{key} is asserted to return 501 (not 200) even for
#     tokens that have the right ability: 501 here means "the ability
#     check passed and the request reached the handler," which is
#     exactly what this script is trying to prove. If you *did* set
#     APP_MASTER_KEY, the write:sensitive/root secrets checks below will
#     fail because they will get 204 instead of 501; that is a
#     limitation of this script, not a real bug.
#
# How to run:
#   APP_DEV_MODE=1 go run ./cmd/levelrail
#   # in another terminal:
#   scripts/dev-auth-check.sh
#
# Env vars:
#   LEVELRAIL_URL       base URL of the running server (default http://localhost:8080)
#   DEV_FIXTURES_FILE   path to dev-fixtures.yml (default <repo root>/dev-fixtures.yml,
#                       resolved relative to this script's location, not the caller's cwd)
#
# Exit code: 0 if every check passed, nonzero otherwise.

set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

LEVELRAIL_URL="${LEVELRAIL_URL:-http://localhost:8080}"
FIXTURES_FILE="${DEV_FIXTURES_FILE:-$REPO_ROOT/dev-fixtures.yml}"

APP_NAME="devauthcheck-web"
APP_IMAGE="nginx:alpine"
APP_PORT=80

if [ ! -f "$FIXTURES_FILE" ]; then
  echo "[ERROR] dev-fixtures.yml not found at $FIXTURES_FILE" >&2
  echo "[ERROR] this script only works against a dev server seeded from that file; see the header comment for what it assumes" >&2
  exit 1
fi

# --- fixture token parsing -------------------------------------------
#
# dev-fixtures.yml is a small, deliberately simple YAML file (see its
# own header comment): a top-level "tokens:" list, each entry a
# "- name: ...", "plaintext: ...", "abilities: [...]" block in that
# fixed order. We use yq if it's on PATH (the correct way to parse
# YAML), and fall back to a plain awk scan of that known simple shape
# otherwise, since yq is not guaranteed to be installed on a bare dev
# machine and this script should not introduce a new dependency for it.
# The fallback is intentionally not a general YAML parser: it only
# understands "find the block whose 'name:' value matches, then read
# that block's 'plaintext:' value," which is all dev-fixtures.yml's
# fixed structure requires.
get_plaintext() {
  local fixture_name="$1"
  if command -v yq >/dev/null 2>&1; then
    yq eval ".tokens[] | select(.name == \"$fixture_name\") | .plaintext" "$FIXTURES_FILE" 2>/dev/null
  else
    awk -v name="$fixture_name" '
      /^[[:space:]]*-[[:space:]]*name:/ {
        val = $0
        sub(/^[[:space:]]*-[[:space:]]*name:[[:space:]]*/, "", val)
        found = (val == name)
      }
      found && /plaintext:/ {
        val = $0
        sub(/^[[:space:]]*plaintext:[[:space:]]*/, "", val)
        print val
        exit
      }
    ' "$FIXTURES_FILE"
  fi
}

READ_TOKEN="$(get_plaintext dev-read)"
WRITE_TOKEN="$(get_plaintext dev-write)"
WRITE_SENSITIVE_TOKEN="$(get_plaintext dev-write-sensitive)"
DEPLOY_TOKEN="$(get_plaintext dev-deploy)"
ROOT_TOKEN="$(get_plaintext dev-root)"

for pair in "dev-read:$READ_TOKEN" "dev-write:$WRITE_TOKEN" "dev-write-sensitive:$WRITE_SENSITIVE_TOKEN" "dev-deploy:$DEPLOY_TOKEN" "dev-root:$ROOT_TOKEN"; do
  fixture_name="${pair%%:*}"
  plaintext="${pair#*:}"
  if [ -z "$plaintext" ]; then
    echo "[ERROR] could not read plaintext for fixture token \"$fixture_name\" from $FIXTURES_FILE" >&2
    echo "[ERROR] has dev-fixtures.yml been edited to remove or rename it?" >&2
    exit 1
  fi
done

# --- request helper ----------------------------------------------------

# http_status METHOD PATH TOKEN [JSON_BODY]
# Prints the response status code only. TOKEN may be empty for an
# unauthenticated request. No response body is parsed anywhere in this
# script, status codes alone are enough to prove the ability boundary,
# so there's no jq/python3 JSON-parsing dependency to worry about.
http_status() {
  local method="$1" path="$2" token="$3" data="${4:-}"
  local curl_args=(-s -o /dev/null -w "%{http_code}" -X "$method")
  if [ -n "$token" ]; then
    curl_args+=(-H "Authorization: Bearer $token")
  fi
  if [ -n "$data" ]; then
    curl_args+=(-H "Content-Type: application/json" -d "$data")
  fi
  curl_args+=("${LEVELRAIL_URL}${path}")
  curl "${curl_args[@]}"
}

# --- pass/fail bookkeeping ---------------------------------------------

PASS_COUNT=0
FAIL_COUNT=0

# assert_status DESCRIPTION EXPECTED ACTUAL [NOTE]
assert_status() {
  local description="$1" expected="$2" actual="$3" note="${4:-}"
  if [ "$actual" = "$expected" ]; then
    PASS_COUNT=$((PASS_COUNT + 1))
    if [ -n "$note" ]; then
      echo "[PASS] $description -> $actual ($note)"
    else
      echo "[PASS] $description -> $actual"
    fi
  else
    FAIL_COUNT=$((FAIL_COUNT + 1))
    echo "[FAIL] $description -> got $actual, want $expected"
  fi
}

# --- cleanup -------------------------------------------------------------
#
# Runs via trap so it fires even if a middle assertion fails (this
# script never exits early on a failed assertion, but a trap is the
# right tool regardless, in case curl itself errors out or the script
# is interrupted). Best effort: a failed cleanup delete is reported but
# does not change the script's own exit code, since the check matrix
# above is what this script exists to validate, not app lifecycle
# management. Idempotent: DELETE on an app that no longer exists is a
# harmless 404, not treated as an error here.
cleanup() {
  curl -s -o /dev/null -X DELETE \
    -H "Authorization: Bearer $ROOT_TOKEN" \
    "${LEVELRAIL_URL}/api/v1/apps/${APP_NAME}"
}
trap cleanup EXIT

# Pre-clean: in case a previous run left the throwaway app behind (e.g.
# the script was killed mid-run before its own cleanup ran), delete it
# up front so the "write token POST /apps -> 201" check below doesn't
# spuriously fail with a 409 conflict against a leftover app.
curl -s -o /dev/null -X DELETE \
  -H "Authorization: Bearer $ROOT_TOKEN" \
  "${LEVELRAIL_URL}/api/v1/apps/${APP_NAME}"

echo "Running dev auth checks against $LEVELRAIL_URL (fixtures: $FIXTURES_FILE)"
echo

CREATE_BODY=$(printf '{"name":"%s","image":"%s","port":%s}' "$APP_NAME" "$APP_IMAGE" "$APP_PORT")
SECRET_BODY='{"value":"sk-devauthcheck-test"}'
SECRET_PATH="/api/v1/apps/${APP_NAME}/secrets/TEST_KEY"

# 1. No token at all on a route that requires an ability -> 401.
status=$(http_status GET "/api/v1/apps" "")
assert_status "unauthenticated GET /apps" 401 "$status"

# 2. read token can list apps.
status=$(http_status GET "/api/v1/apps" "$READ_TOKEN")
assert_status "read token GET /apps" 200 "$status"

# 3. read token cannot create an app.
status=$(http_status POST "/api/v1/apps" "$READ_TOKEN" "$CREATE_BODY")
assert_status "read token POST /apps" 403 "$status"

# 4. write token cannot list apps either: abilities are not hierarchical
#    beyond root, write does not imply read.
status=$(http_status GET "/api/v1/apps" "$WRITE_TOKEN")
assert_status "write token GET /apps (write does not imply read)" 403 "$status"

# 5. write token can create an app. This is the throwaway app the rest
#    of the script (and cleanup) operates on.
status=$(http_status POST "/api/v1/apps" "$WRITE_TOKEN" "$CREATE_BODY")
assert_status "write token POST /apps (create $APP_NAME)" 201 "$status"

# 6. write token cannot set a secret, only write:sensitive can.
status=$(http_status PUT "$SECRET_PATH" "$WRITE_TOKEN" "$SECRET_BODY")
assert_status "write token PUT secrets" 403 "$status"

# 7. write:sensitive token reaches the secrets handler. It gets 501, not
#    200, because a bare local dev server (per this script's own
#    assumptions above) has no APP_MASTER_KEY set, so
#    internal/api.Router has no SecretSetter configured and
#    handleSetSecret returns 501 before ever touching the app. 501 here
#    is the "ability check passed, request reached the handler" signal,
#    not a failure.
status=$(http_status PUT "$SECRET_PATH" "$WRITE_SENSITIVE_TOKEN" "$SECRET_BODY")
assert_status "write:sensitive token PUT secrets" 501 "$status" "no local master key configured, this means the ability check passed"

# 8. deploy token cannot list apps: deploy does not imply read either.
status=$(http_status GET "/api/v1/apps" "$DEPLOY_TOKEN")
assert_status "deploy token GET /apps (deploy does not imply read)" 403 "$status"

# 9. root token implies every ability: it can list apps...
status=$(http_status GET "/api/v1/apps" "$ROOT_TOKEN")
assert_status "root token GET /apps" 200 "$status"

# 10. ...and it can reach the secrets handler too (same 501 caveat as
#     check 7).
status=$(http_status PUT "$SECRET_PATH" "$ROOT_TOKEN" "$SECRET_BODY")
assert_status "root token PUT secrets" 501 "$status" "no local master key configured, this means the ability check passed"

echo
echo "$PASS_COUNT/$((PASS_COUNT + FAIL_COUNT)) checks passed"

if [ "$FAIL_COUNT" -gt 0 ]; then
  exit 1
fi
exit 0
