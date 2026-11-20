#!/usr/bin/env bash
# Real-host proof: git push deploy with a real BuildKit build, served on an
# sslip.io host with a real certificate. Safe to re-run; the throwaway app and
# its remote are removed on exit and the git-push beta switch is put back.
#
# Required env:
#   LEVELRAIL_API_URL   HTTPS dashboard address of the host under test
#   LEVELRAIL_TOKEN     API token with root or write access (short lived)
# Optional env:
#   PROOF_APP           throwaway app name (default proof-gitpush-HHMMSS, a stored push copy outlives an app delete)
#   PROOF_BASE_IMAGE    image the empty app is created from (default nginx:alpine)
#   PROOF_WAIT_SECONDS  wait for each stage (default 240)
set -euo pipefail

: "${LEVELRAIL_API_URL:?set LEVELRAIL_API_URL}"
: "${LEVELRAIL_TOKEN:?set LEVELRAIL_TOKEN}"
APP="${PROOF_APP:-proof-gitpush-$(date +%H%M%S)}"
BASE_IMAGE="${PROOF_BASE_IMAGE:-nginx:alpine}"
WAIT="${PROOF_WAIT_SECONDS:-240}"
API="${LEVELRAIL_API_URL%/}"
WORK="$(mktemp -d)"
MARK="gitpush-proof-$(date +%s)-$RANDOM"
FEATURE_WAS=""
APP_CREATED=0
FAIL=0

now() { date +%s.%N; }
elapsed() { awk -v a="$1" -v b="$2" 'BEGIN{printf "%.1f", b-a}'; }
log() { printf '%s\n' "proof-gitpush: $*"; }
pass() { log "PASS $*"; }
fail() { log "FAIL $*"; FAIL=1; }

api() {
  local method="$1" path="$2" body="${3:-}"
  if [ -n "$body" ]; then
    curl -sS -m 60 -X "$method" -H "Authorization: Bearer $LEVELRAIL_TOKEN" -H 'Content-Type: application/json' -d "$body" -w '\n%{http_code}' "$API$path"
  else
    curl -sS -m 60 -X "$method" -H "Authorization: Bearer $LEVELRAIL_TOKEN" -w '\n%{http_code}' "$API$path"
  fi
}

cleanup() {
  set +e
  log "cleanup"
  if [ "$APP_CREATED" = 1 ]; then
    api DELETE "/api/v1/apps/$APP/git-remote" >/dev/null
    # A push deploy leaves a retained previous-deploy app named APP-dep<id>; deleting APP alone does not remove it.
    for sib in $(curl -sS -m 30 -H "Authorization: Bearer $LEVELRAIL_TOKEN" "$API/api/v1/apps" | jq -r --arg p "$APP-dep" '.[] | select(.name | startswith($p)) | .name'); do
      log "removing retained deploy app $sib"
      api DELETE "/api/v1/apps/$sib" >/dev/null
    done
    api DELETE "/api/v1/apps/$APP" >/dev/null
    sleep 8
    if [ -n "${host:-}" ]; then
      log "after delete https://$host/ answers: $(curl -sS -m 10 -o /dev/null -w '%{http_code}' "https://$host/" 2>&1 | tail -c 80)"
    fi
  fi
  if [ "$FEATURE_WAS" = "false" ]; then
    api PUT /api/v1/settings/beta-features/git-push '{"enabled":false}' >/dev/null
  fi
  rm -rf "$WORK"
  exit "$FAIL"
}
trap cleanup EXIT

# 1. Switch git-push on when it is off, remember the old state.
state="$(curl -sS -m 30 -H "Authorization: Bearer $LEVELRAIL_TOKEN" "$API/api/v1/settings/beta-features" | jq -r '.[] | select(.id=="git-push") | .enabled')"
FEATURE_WAS="$state"
if [ "$state" != "true" ]; then
  out="$(api PUT /api/v1/settings/beta-features/git-push '{"enabled":true}')"
  [ "$(tail -n1 <<<"$out")" = 200 ] || { fail "enable git-push: $out"; exit 1; }
fi
log "git-push was enabled=$state"

# 2. Throwaway app from a base image, so the push is what changes it.
t0="$(now)"
out="$(api POST /api/v1/apps "{\"name\":\"$APP\",\"image\":\"$BASE_IMAGE\",\"port\":80}")"
code="$(tail -n1 <<<"$out")"
if [ "$code" != 201 ] && [ "$code" != 200 ]; then fail "create app HTTP $code: $(sed '$d' <<<"$out")"; exit 1; fi
APP_CREATED=1
pass "app $APP created"

host=""
for _ in $(seq 1 30); do
  host="$(curl -sS -m 20 -H "Authorization: Bearer $LEVELRAIL_TOKEN" "$API/api/v1/apps/$APP/auto-host" | jq -r '.hostname // empty')"
  [ -n "$host" ] && break
  sleep 2
done
[ -n "$host" ] || { fail "no automatic hostname for $APP"; exit 1; }
log "hostname $host"

# 3. Issue the remote.
out="$(api POST "/api/v1/apps/$APP/git-remote")"
[ "$(tail -n1 <<<"$out")" = 200 ] || { fail "issue remote: $(sed '$d' <<<"$out" | sed -E 's/gp_[A-Za-z0-9_-]+/gp_REDACTED/g')"; exit 1; }
body="$(sed '$d' <<<"$out")"
token="$(jq -r .token <<<"$body")"
url="$(jq -r .url <<<"$body")"
branch="$(jq -r .branch <<<"$body")"
pass "remote issued, url $url, deploy branch $branch"

# 4. Tiny repo with a Dockerfile that copies a marker page.
mkdir "$WORK/repo"
cd "$WORK/repo"
git init -q -b "$branch"
cat >Dockerfile <<EOF
FROM $BASE_IMAGE
COPY index.html /usr/share/nginx/html/index.html
EOF
printf '%s\n' "$MARK" >index.html
git add Dockerfile index.html
git -c user.name=proof -c user.email=proof@example.invalid commit -q -m "proof build"
commit="$(git rev-parse --short HEAD)"

# 5. Push over HTTPS with the token supplied through a one-shot credential helper.
t1="$(now)"
push_log="$WORK/push.log"
GIT_TERMINAL_PROMPT=0 LEVELRAIL_GIT_TOKEN="$token" git \
  -c credential.helper= \
  -c 'credential.helper=!f() { echo username=git; echo "password=$LEVELRAIL_GIT_TOKEN"; }; f' \
  push "$url" "HEAD:refs/heads/$branch" >"$push_log" 2>&1 && pushrc=0 || pushrc=$?
t2="$(now)"
sed -E 's/gp_[A-Za-z0-9_-]+/gp_REDACTED/g' "$push_log" | tail -n 25
if [ "$pushrc" = 0 ] && grep -q "Deploy succeeded" "$push_log"; then
  pass "push and deploy succeeded in $(elapsed "$t1" "$t2")s (commit $commit)"
else
  fail "push exit $pushrc"
  exit 1
fi

# 6. Serve check on the real hostname with certificate verification on.
t3="$(now)"
served=""
for _ in $(seq 1 "$((WAIT / 3))"); do
  served="$(curl -sS -m 10 "https://$host/" 2>/dev/null || true)"
  grep -q "$MARK" <<<"$served" && break
  sleep 3
done
t4="$(now)"
if grep -q "$MARK" <<<"$served"; then
  pass "https://$host/ serves the pushed build, first answer $(elapsed "$t3" "$t4")s after the push finished"
else
  fail "marker not served on https://$host/"
  exit 1
fi
verify="$(curl -sS -m 10 -o /dev/null -w '%{ssl_verify_result} %{http_code}' "https://$host/")"
if [ "$verify" = "0 200" ]; then pass "certificate verifies, HTTP 200"; else fail "tls/http result $verify"; fi
log "cert: $(echo | openssl s_client -connect "$host:443" -servername "$host" 2>/dev/null | openssl x509 -noout -issuer -enddate 2>/dev/null | tr '\n' ' ')"
log "http redirect: $(curl -sS -m 10 -o /dev/null -w '%{http_code}' "http://$host/")"

# 7. The deploy is in history with the commit.
out="$(api GET "/api/v1/apps/$APP/deploy-attempts")"
hist="$(sed '$d' <<<"$out" | jq -r --arg c "$commit" '[.. | strings | select(startswith($c))] | length' 2>/dev/null || echo '?')"
log "deploy history HTTP $(tail -n1 <<<"$out"), entries mentioning commit $commit: $hist"
sed '$d' <<<"$out" | jq -c 'if type=="array" then .[0] else . end | keys' 2>/dev/null | head -c 400 || true
log "timings: push+build+deploy $(elapsed "$t1" "$t2")s, total from app create $(elapsed "$t0" "$t4")s"
exit "$FAIL"
