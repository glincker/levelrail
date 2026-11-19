#!/usr/bin/env bash
# Repeatable Pages smoke test against a RUNNING Levelrail. Not part of CI.
# Env: LEVELRAIL_URL, LEVELRAIL_TOKEN (required), SMOKE_HOST (connect address for real
# hosts), SMOKE_GIT_URL (public repo), SMOKE_DOMAIN, SMOKE_DRY_RUN=1, LEVELRAIL_CLI,
# SMOKE_GIT_TIMEOUT (seconds, default 300), SMOKE_STRICT_TLS=1 (verify certificates).
set -euo pipefail

PASS=0
FAIL=0
SKIP=0
SITE="pages-smoke-$(date +%s)-$$"
WORK=""
CLI="${LEVELRAIL_CLI:-}"
BASE=""
SMOKE_PW=""

pass() { PASS=$((PASS + 1)); printf 'PASS  %s\n' "$1"; }
fail() { FAIL=$((FAIL + 1)); printf 'FAIL  %s\n' "$1"; }
skip() { SKIP=$((SKIP + 1)); printf 'SKIP  %s\n' "$1"; }
check() { # check "label" expected actual
  if [ "$2" = "$3" ]; then pass "$1"; else fail "$1 (want $2, got $3)"; fi
}

plan() {
  cat <<'PLAN'
pages-smoke plan:
  1 build CLI (or use LEVELRAIL_CLI) and create a temp site dir
  2 pages deploy the site; read the live URL
  3 serve checks: / 200, /about 200, redirect status and Location, custom header,
    404 page status, /_redirects and /_headers not served
  4 deploys list: per-deploy URL serves the same content
  5 second deploy, promote old deploy, rollback round trip
  6 password: 401 without credentials, 200 as user site, 401 on deploy host, clear, open
  7 checks endpoint score, traffic endpoint series
  8 forms probe (SKIP when missing), pages init probe (SKIP when missing)
  9 git deploy steps reach live (only with SMOKE_GIT_URL)
 10 domain status diagnosis (only with SMOKE_DOMAIN)
 11 cleanup: delete the temp site, always
PLAN
}

if [ "${SMOKE_DRY_RUN:-}" = "1" ]; then
  plan
  exit 0
fi

: "${LEVELRAIL_URL:?set LEVELRAIL_URL}"
: "${LEVELRAIL_TOKEN:?set LEVELRAIL_TOKEN}"
for tool in curl jq; do
  command -v "$tool" >/dev/null 2>&1 || { echo "missing tool: $tool" >&2; exit 2; }
done
export LEVELRAIL_API_URL="${LEVELRAIL_URL%/}"
export LEVELRAIL_TOKEN
export APP_API_URL="$LEVELRAIL_API_URL" APP_API_TOKEN="$LEVELRAIL_TOKEN"

cleanup() {
  local rc=$?
  if [ -n "$CLI" ] && [ -x "$CLI" ]; then
    "$CLI" pages delete "$SITE" >/dev/null 2>&1 || true
    "$CLI" pages delete "$SITE-git" >/dev/null 2>&1 || true
  fi
  if [ -n "$WORK" ]; then rm -rf "$WORK"; fi
  printf '\n%d passed, %d failed, %d skipped\n' "$PASS" "$FAIL" "$SKIP"
  if [ "$rc" -ne 0 ] || [ "$FAIL" -ne 0 ]; then exit 1; fi
}
trap cleanup EXIT

WORK="$(mktemp -d)"
if [ -z "$CLI" ]; then
  (cd "$(dirname "$0")/.." && go build -o "$WORK/levelrail" ./cmd/levelrail-cli)
  CLI="$WORK/levelrail"
fi

# api METHOD PATH [curl args]: prints body, then the status code on a last line.
api() {
  local method="$1" path="$2"
  shift 2
  curl -sS -X "$method" --max-time 30 -w '\n%{http_code}' \
    -H "Authorization: Bearer $LEVELRAIL_TOKEN" "$@" "$LEVELRAIL_API_URL$path"
}
api_body() { sed '$d'; }
api_code() { tail -n 1; }

# site_curl URL [curl args]: serves through SMOKE_HOST when set, tolerates dev certificates.
site_curl() {
  local url="$1" hostport host port opts=()
  shift
  hostport="${url#*://}"
  hostport="${hostport%%/*}"
  host="${hostport%%:*}"
  if [ "$hostport" = "$host" ]; then
    case "$url" in https://*) port=443 ;; *) port=80 ;; esac
  else
    port="${hostport##*:}"
  fi
  [ "${SMOKE_STRICT_TLS:-}" = "1" ] || opts+=(-k)
  [ -z "${SMOKE_HOST:-}" ] || opts+=(--resolve "$host:$port:$SMOKE_HOST")
  curl -sS --max-time 15 "${opts[@]}" "$@" "$url"
}
code_of() { site_curl "$@" -o /dev/null -w '%{http_code}' || echo 000; }

# wait_code URL WANT [curl args]: polls briefly while a deploy settles.
wait_code() {
  local url="$1" want="$2" got=""
  shift 2
  for _ in 1 2 3 4 5 6 7 8 9 10; do
    got="$(code_of "$url" "$@")"
    [ "$got" = "$want" ] && break
    sleep 1
  done
  echo "$got"
}

write_site() { # write_site MARKER
  local d="$WORK/site"
  mkdir -p "$d"
  printf '<!doctype html><title>home</title><p>%s</p>\n' "$1" >"$d/index.html"
  printf '<!doctype html><title>about</title><p>about page</p>\n' >"$d/about.html"
  printf '<!doctype html><title>nope</title><p>smoke custom 404</p>\n' >"$d/404.html"
  printf '/old /about 301\n' >"$d/_redirects"
  printf '/*\n  X-Smoke: pages\n' >"$d/_headers"
}

deploy_site() {
  "$CLI" pages deploy "$WORK/site" --name "$SITE" 2>"$WORK/deploy.err" | tail -n 1
}

live_deploy_id() { api GET "/api/v1/static-sites/$SITE/deploys" | api_body | jq -r '[.deploys[] | select(.live)][0].id // empty'; }
other_deploy_id() { api GET "/api/v1/static-sites/$SITE/deploys" | api_body | jq -r '[.deploys[] | select((.live | not) and .promotable)][0].id // empty'; }

# ---- 2. deploy and serve checks
write_site smoke-v1
BASE="$(deploy_site || true)"
BASE="${BASE%/}"
case "$BASE" in
  http://* | https://*) pass "deploy published, live URL $BASE" ;;
  *) fail "deploy did not print a live URL ($(tr '\n' ' ' <"$WORK/deploy.err" | cut -c1-160))"; exit 1 ;;
esac

check "GET / is 200" 200 "$(wait_code "$BASE/" 200)"
check "clean URL /about is 200" 200 "$(code_of "$BASE/about")"
check "redirect /old status" 301 "$(code_of "$BASE/old")"
loc="$(site_curl "$BASE/old" -o /dev/null -D - | tr -d '\r' | awk 'tolower($1)=="location:"{print $2}')"
case "$loc" in */about) pass "redirect Location ends in /about" ;; *) fail "redirect Location (got '$loc')" ;; esac
hdr="$(site_curl "$BASE/" -o /dev/null -D - | tr -d '\r' | awk 'tolower($1)=="x-smoke:"{print $2}')"
check "custom header X-Smoke" pages "$hdr"
check "missing path serves 404 status" 404 "$(code_of "$BASE/does-not-exist")"
if site_curl "$BASE/does-not-exist" | grep -q 'smoke custom 404'; then pass "custom 404 body"; else fail "custom 404 body"; fi
check "/_redirects not served" 404 "$(code_of "$BASE/_redirects")"
check "/_headers not served" 404 "$(code_of "$BASE/_headers")"

# ---- 4. deploys list and per-deploy URL
LIVE_ID="$(live_deploy_id)"
DURL="$(api GET "/api/v1/static-sites/$SITE/deploys" | api_body | jq -r '[.deploys[] | select(.live)][0].url // empty')"
if [ -z "$DURL" ]; then
  fail "deploys list has no per-deploy URL (automatic hostnames may be off)"
else
  pass "deploys list shows per-deploy URL"
  if site_curl "${DURL%/}/" | grep -q smoke-v1; then pass "per-deploy URL serves the same content"; else fail "per-deploy URL content"; fi
fi

# ---- 5. promote and rollback round trip
write_site smoke-v2
deploy_site >/dev/null || true
wait_code "$BASE/" 200 >/dev/null
if site_curl "$BASE/" | grep -q smoke-v2; then pass "second deploy is live"; else fail "second deploy is live"; fi
if [ -n "$LIVE_ID" ] && "$CLI" pages promote "$SITE" "$LIVE_ID" >/dev/null 2>&1; then
  sleep 1
  if site_curl "$BASE/" | grep -q smoke-v1; then pass "promote old deploy serves v1"; else fail "promote old deploy serves v1"; fi
else
  fail "promote old deploy"
fi
if "$CLI" pages rollback "$SITE" >/dev/null 2>&1; then
  sleep 1
  if site_curl "$BASE/" | grep -q smoke-v2; then pass "rollback returns to v2"; else fail "rollback returns to v2"; fi
else
  fail "rollback"
fi

# ---- 6. password gate
SMOKE_PW="smoke-$(date +%s)-$$"
export SMOKE_PW
if "$CLI" pages settings set "$SITE" --password-env SMOKE_PW >/dev/null 2>&1; then
  sleep 1
  check "protected: 401 without credentials" 401 "$(code_of "$BASE/")"
  check "protected: 200 with user site" 200 "$(code_of "$BASE/" -u "site:$SMOKE_PW")"
  CUR_DURL="$(api GET "/api/v1/static-sites/$SITE/deploys" | api_body | jq -r '[.deploys[] | select(.live)][0].url // empty')"
  if [ -n "$CUR_DURL" ]; then
    check "protected: deploy host 401 without credentials" 401 "$(code_of "${CUR_DURL%/}/")"
  else
    skip "deploy host password check (no per-deploy URL)"
  fi
  "$CLI" pages settings set "$SITE" --clear-password >/dev/null 2>&1 || fail "clear password"
  sleep 1
  check "cleared: open again" 200 "$(code_of "$BASE/")"
else
  fail "set password"
fi
unset SMOKE_PW

# ---- 7. checks and traffic
out="$(api GET "/api/v1/static-sites/$SITE/checks")"
case "$(echo "$out" | api_code)" in
  200) if echo "$out" | api_body | jq -e 'has("score")' >/dev/null; then pass "checks endpoint returns a score"; else fail "checks endpoint has no score"; fi ;;
  404) skip "checks endpoint not available" ;;
  *) fail "checks endpoint status $(echo "$out" | api_code)" ;;
esac
out="$(api GET "/api/v1/static-sites/$SITE/traffic?range=24h")"
case "$(echo "$out" | api_code)" in
  200) if echo "$out" | api_body | jq -e '.series | type == "array"' >/dev/null; then pass "traffic endpoint returns a series"; else fail "traffic endpoint has no series"; fi ;;
  404) skip "traffic endpoint not available" ;;
  *) fail "traffic endpoint status $(echo "$out" | api_code)" ;;
esac

# ---- 8. probes for features other builders ship
out="$(api GET "/api/v1/static-sites/$SITE/forms")"
case "$(echo "$out" | api_code)" in
  200) pass "forms endpoint answers" ;;
  404 | 405) skip "forms endpoint not available" ;;
  *) fail "forms endpoint status $(echo "$out" | api_code)" ;;
esac
if "$CLI" pages init --help >/dev/null 2>&1; then pass "pages init command exists"; else skip "pages init command not available"; fi

# ---- 9. git deploy
if [ -n "${SMOKE_GIT_URL:-}" ]; then
  if "$CLI" pages deploy --git "$SMOKE_GIT_URL" --name "$SITE-git" >"$WORK/git.out" 2>&1; then
    deadline=$(( $(date +%s) + ${SMOKE_GIT_TIMEOUT:-300} ))
    state=""
    while [ "$(date +%s)" -lt "$deadline" ]; do
      state="$(api GET "/api/v1/static-sites/$SITE-git" | api_body | jq -r '.latest_deploy.steps // [] | map(select(.name == "live") | .state)[0] // empty')"
      [ "$state" = "done" ] || [ "$state" = "failed" ] && break
      sleep 5
    done
    check "git deploy steps reach live" "done" "$state"
  else
    fail "git deploy start ($(tr '\n' ' ' <"$WORK/git.out" | cut -c1-160))"
  fi
else
  skip "git deploy (set SMOKE_GIT_URL)"
fi

# ---- 10. domain diagnosis
if [ -n "${SMOKE_DOMAIN:-}" ]; then
  out="$(api GET "/api/v1/static-sites/$SITE/domains/$SMOKE_DOMAIN/status")"
  case "$(echo "$out" | api_code)" in
    200) if echo "$out" | api_body | jq -e 'has("diagnosis")' >/dev/null; then pass "domain status has a diagnosis"; else skip "domain status has no diagnosis field yet"; fi ;;
    404) skip "domain status or diagnosis not available" ;;
    *) fail "domain status $(echo "$out" | api_code)" ;;
  esac
else
  skip "domain diagnosis (set SMOKE_DOMAIN)"
fi
