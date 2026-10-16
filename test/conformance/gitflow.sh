#!/usr/bin/env bash
# Git push-to-deploy conformance: real commits on the lr-qa fixture repos, replayed
# to the plane as signed GitHub push deliveries (a localhost plane cannot be reached
# by github.com). Apps are named qa-gitflow-* and left running.
# Usage: APP_PROFILE=<profile> GIT_TOKEN=$(gh auth token) test/conformance/gitflow.sh
#   [--only 1,3,7] [--timeout 240] [--run-id ID] [--skip-monorepo] [--i-know]
# Env: LEVELRAIL_CLI, APP_API_URL, APP_API_TOKEN (or credentials profile). bash 3.2 safe.
set -u

HERE=$(cd "$(dirname "$0")" && pwd)
REPO_ROOT=$(cd "$HERE/../.." && pwd)
ALLOWED="$HERE/allowed-hosts"
OUT_DIR="$HERE/results"

ONLY=""; WAIT=240; RUN_ID=""; I_KNOW=0; SKIP_MONO=0
while [ $# -gt 0 ]; do
  case "$1" in
    --only) ONLY="${2:-}"; shift 2 ;;
    --timeout) WAIT="${2:-240}"; shift 2 ;;
    --run-id) RUN_ID="${2:-}"; shift 2 ;;
    --skip-monorepo) SKIP_MONO=1; shift ;;
    --i-know) I_KNOW=1; shift ;;
    -h|--help) sed -n 2,7p "$0" | sed 's/^# \{0,1\}//'; exit 0 ;;
    *) echo "unknown flag: $1" >&2; exit 2 ;;
  esac
done

for tool in jq curl git openssl; do
  command -v "$tool" >/dev/null 2>&1 || { echo "missing tool: $tool" >&2; exit 2; }
done
[ -n "${GIT_TOKEN:-}" ] || { echo "GIT_TOKEN is required (push access to glincker/lr-qa-*)" >&2; exit 2; }

TMP=$(mktemp -d "${TMPDIR:-/tmp}/lr-gitflow.XXXXXX")
trap 'rm -rf "$TMP"' EXIT

if [ -n "${LEVELRAIL_CLI:-}" ]; then
  CLI="$LEVELRAIL_CLI"
else
  CLI="$TMP/levelrail-cli"
  echo "building CLI..." >&2
  (cd "$REPO_ROOT" && go build -o "$CLI" ./cmd/levelrail-cli) || { echo "CLI build failed" >&2; exit 2; }
fi

cred_value() {
  local file want
  file="${XDG_CONFIG_HOME:-$HOME/.config}/levelrail-cli/credentials"
  want="${APP_PROFILE:-default}"
  [ -f "$file" ] || return 0
  awk -v want="$want" -v key="$1" '
    /^\[/ { cur=$0; gsub(/[\[\]]/,"",cur); next }
    cur==want && index($0, key "=")==1 { sub(key "=",""); print; exit }' "$file"
}
API_URL="${APP_API_URL:-$(cred_value APP_API_URL)}"
API_URL="${API_URL:-http://localhost:8080}"
API_URL="${API_URL%/}"
API_TOKEN="${APP_API_TOKEN:-$(cred_value APP_API_TOKEN)}"
export APP_API_URL="$API_URL"
[ -n "$API_TOKEN" ] && export APP_API_TOKEN="$API_TOKEN"
API_HOST=$(printf '%s' "$API_URL" | sed -e 's#^[a-zA-Z]*://##' -e 's#[/?].*$##' -e 's#^.*@##' -e 's#:[0-9]*$##')
case "$API_HOST" in
  localhost|127.0.0.1) ;;
  *)
    if ! grep -v '^[[:space:]]*#' "$ALLOWED" 2>/dev/null | tr -d '[:blank:]' | grep -qxF -- "$API_HOST" || [ "$I_KNOW" -ne 1 ]; then
      echo "refusing: gitflow.sh only targets a local plane unless the host is allowed and --i-know is set" >&2
      exit 3
    fi ;;
esac

TS=$(date +%Y%m%d-%H%M%S)
[ -n "$RUN_ID" ] || RUN_ID="gitflow-$TS"
TAG="g$(date +%s)"
ROWS="$TMP/rows.jsonl"; : >"$ROWS"
EVF=""
OUT=""; RC=0
HC=""; HB=""

EXP_APP="qa-gitflow-express"
MONO_APP="qa-gitflow-monorepo-paths"
EXP_REPO="glincker/lr-qa-express"
MONO_REPO="glincker/lr-qa-monorepo-paths"
EXP_SECRET=""; MONO_SECRET=""

now() { date +%s; }
ev() { [ -n "$EVF" ] && printf '%s\n' "$*" >>"$EVF"; return 0; }
begin() { EVF="$TMP/ev.$1"; : >"$EVF"; echo "==> $1: $2" >&2; }
selected() { [ -z "$ONLY" ] && return 0; case ",$ONLY," in *",$1,"*) return 0 ;; esac; return 1; }
# verdict ID NAME PASS|FAIL|NOTE EXPECTED ACTUAL
verdict() {
  local e
  e=$(head -c 5000 "$EVF" 2>/dev/null)
  jq -nc --arg id "$1" --arg n "$2" --arg v "$3" --arg x "$4" --arg a "$5" --arg e "$e" \
    '{id:$id,scenario:$n,verdict:$v,expected:$x,actual:$a,evidence:$e}' >>"$ROWS"
  echo "    $3: $5" >&2
}
cli() { OUT=$("$CLI" "$@" 2>&1); RC=$?; return 0; }
redact() { sed -e "s/${GIT_TOKEN}/***/g"; }

api_get() { curl -s -m 20 -H "Authorization: Bearer $API_TOKEN" "$API_URL$1"; }

# The CLI's "apps deploys list --json" drops branch, author and commit_message, so read the API.
deploys_json() { api_get "/api/v1/apps/$1/deploy-attempts" | jq -c 'if type=="array" then . else (.attempts // []) end' 2>/dev/null || echo '[]'; }
attempt_count() { deploys_json "$1" | jq 'length'; }
attempts_for() { deploys_json "$1" | jq -c --arg s "$2" '[.[] | select(.commit_sha==$s)]'; }
host_port() { "$CLI" apps network "$1" --json 2>/dev/null | jq -r '.host_port // empty'; }

# wait_sha APP SHA: poll until an attempt for SHA reaches a final status. Sets W_STATUS, W_ATTEMPT.
W_STATUS=""; W_ATTEMPT=""
wait_sha() {
  local end row st
  end=$(($(now) + WAIT)); W_STATUS="timeout"; W_ATTEMPT=""
  while [ "$(now)" -lt "$end" ]; do
    row=$(attempts_for "$1" "$2" | jq -c '.[0] // empty')
    if [ -n "$row" ]; then
      st=$(printf '%s' "$row" | jq -r '.status')
      W_ATTEMPT=$(printf '%s' "$row" | jq -r '.id')
      case "$st" in
        succeeded|failed|canceled|superseded) W_STATUS="$st"; return 0 ;;
      esac
    fi
    sleep 3
  done
  return 1
}

# wait_body APP NEEDLE [SECONDS]: poll the app's host port until the body contains NEEDLE.
BODY=""
wait_body() {
  local end p body
  end=$(($(now) + ${3:-90})); BODY=""
  while [ "$(now)" -lt "$end" ]; do
    p=$(host_port "$1")
    if [ -n "$p" ]; then
      body=$(curl -s -m 5 "http://127.0.0.1:${p}/" 2>/dev/null)
      BODY="$body"
      case "$body" in *"$2"*) return 0 ;; esac
    fi
    sleep 3
  done
  return 1
}

# ---------------------------------------------------------------- git
GIT_DIR_ROOT="$TMP/git"; mkdir -p "$GIT_DIR_ROOT"
git_clone() { # slug dir
  git clone -q "https://x-access-token:${GIT_TOKEN}@github.com/$1.git" "$2" 2>&1 | redact >&2
  [ -d "$2/.git" ] || return 1
  git -C "$2" config user.name "lr-qa-gitflow"
  git -C "$2" config user.email "lr-qa-gitflow@users.noreply.github.com"
}
# edit_commit DIR FILE SEDEXPR MSG: apply a sed edit, commit, print the new SHA on stdout.
edit_commit() {
  sed -E -i.bak "$3" "$1/$2" && rm -f "$1/$2.bak"
  if git -C "$1" diff --quiet; then echo "edit produced no change in $2" >&2; return 1; fi
  git -C "$1" commit -q -am "$4" && git -C "$1" rev-parse HEAD
}
push_ref() { git -C "$1" push -q "$2" "$3" 2>&1 | redact >&2; }

# ---------------------------------------------------------------- webhook send
# payload REF BEFORE AFTER FULLNAME MSG ADDED MODIFIED REMOVED [FORCED]  (lists are JSON arrays)
payload() {
  jq -nc --arg ref "$1" --arg before "$2" --arg after "$3" --arg full "$4" --arg msg "$5" \
    --argjson added "$6" --argjson modified "$7" --argjson removed "$8" --argjson forced "${9:-false}" '
    {ref:$ref, before:$before, after:$after, created:false, deleted:false, forced:$forced,
     repository:{full_name:$full, name:($full|split("/")[1]), clone_url:("https://github.com/"+$full+".git"),
       html_url:("https://github.com/"+$full), default_branch:"main"},
     pusher:{name:"lr-qa-gitflow", email:"lr-qa-gitflow@users.noreply.github.com"},
     sender:{login:"lr-qa-gitflow"},
     head_commit:{id:$after, message:$msg, timestamp:(now|todate), author:{name:"lr-qa-gitflow", email:"lr-qa-gitflow@users.noreply.github.com"},
       added:$added, modified:$modified, removed:$removed},
     commits:[{id:$after, message:$msg, timestamp:(now|todate), author:{name:"lr-qa-gitflow"},
       added:$added, modified:$modified, removed:$removed}]}'
}
hmac() { openssl dgst -sha256 -hmac "$2" <"$1" | awk '{print $NF}'; }

# send APP SECRET EVENT DELIVERY SIGMODE BODYFILE: SIGMODE good|bad|none. Sets HC, HB.
send() {
  local app="$1" secret="$2" event="$3" delivery="$4" mode="$5" file="$6" sig n=0
  local args
  args=(-s -m "$WAIT" -o "$TMP/resp" -w '%{http_code}' -X POST -H "Content-Type: application/json"
        -H "X-GitHub-Event: $event" -H "X-GitHub-Delivery: $delivery" --data-binary "@$file")
  case "$mode" in
    good) sig=$(hmac "$file" "$secret"); args+=(-H "X-Hub-Signature-256: sha256=$sig") ;;
    bad) args+=(-H "X-Hub-Signature-256: sha256=$(printf '%064d' 0)") ;;
    none) : ;;
  esac
  while :; do
    HC=$(curl "${args[@]}" "$API_URL/api/v1/webhooks/github/$app" 2>/dev/null)
    HB=$(head -c 400 "$TMP/resp" 2>/dev/null | tr '\n' ' ')
    if [ "$HC" = "429" ] && [ $n -lt 4 ]; then n=$((n + 1)); sleep 8; continue; fi
    break
  done
  ev "POST /api/v1/webhooks/github/$app event=$event delivery=$delivery sig=$mode -> HTTP $HC $HB"
}

# ---------------------------------------------------------------- app setup
app_exists() { "$CLI" apps get "$1" --json >/dev/null 2>&1; }

SETUP_SECRET=""; SETUP_URL=""
# setup_app APP REPO PORT BUILDTYPE BASEDIR PATHS
setup_app() {
  local app="$1" repo="$2" port="$3" bt="$4" bd="$5" paths="$6" args
  if ! app_exists "$app"; then
    cli apps create --name "$app" --image "$app:pending" --port "$port" --json
    [ $RC -eq 0 ] || { echo "create $app failed: $OUT" >&2; return 1; }
  fi
  args=(apps git-source set "$app" --repo-url "https://github.com/$repo" --branch main --build-type "$bt" --trigger-mode push --token-secret "$GIT_TOKEN" --json)
  [ -n "$bd" ] && args+=(--base-directory "$bd")
  cli "${args[@]}"
  [ $RC -eq 0 ] || { echo "git-source set $app failed: $(printf '%s' "$OUT" | redact)" >&2; return 1; }
  cli apps git-source settings "$app" --paths "$paths" --paths-ignore "" --json
  [ $RC -eq 0 ] || { echo "git-source settings $app failed: $OUT" >&2; return 1; }
  cli apps git-source rotate-secret "$app" --json
  [ $RC -eq 0 ] || { echo "rotate-secret $app failed: $OUT" >&2; return 1; }
  SETUP_SECRET=$(printf '%s' "$OUT" | jq -r '.webhook_secret // empty')
  SETUP_URL=$(printf '%s' "$OUT" | jq -r '.webhook_url // empty')
  [ -n "$SETUP_SECRET" ]
}

# baseline APP REPO REF BUILDTYPE BASEDIR: one manual build so a previous version exists.
baseline() {
  local args id end st
  args=(apps builds trigger "$1" --repo "https://github.com/$2" --ref "$3" --image-repo "$1" --build-type "$4" --json)
  [ -n "$5" ] && args+=(--base-directory "$5")
  cli "${args[@]}"
  [ $RC -eq 0 ] || { echo "baseline trigger $1 failed: $OUT" >&2; return 1; }
  id=$(printf '%s' "$OUT" | jq -r '.id // empty')
  end=$(($(now) + WAIT))
  while [ "$(now)" -lt "$end" ]; do
    st=$(deploys_json "$1" | jq -r --arg i "$id" '.[] | select(.id==$i) | .status')
    case "$st" in
      succeeded) return 0 ;;
      failed|canceled) return 1 ;;
    esac
    sleep 3
  done
  return 1
}

show_attempt() { # app sha
  ev "deploy attempt(s) for ${2:0:12}: $(attempts_for "$1" "$2" | jq -c '[.[] | {id,status,source,commit_sha,branch,author,commit_message,image}]' | head -c 700)"
}

# ================================================================ setup
echo "run $RUN_ID marker $TAG against $API_URL" >&2
begin 0 "setup: apps, git sources, secrets, baseline deploys"
SETUP_OK=1
EXP_HOOK=""
git_clone "$EXP_REPO" "$GIT_DIR_ROOT/express" || SETUP_OK=0
if setup_app "$EXP_APP" "$EXP_REPO" 3000 railpack "" ""; then
  EXP_SECRET="$SETUP_SECRET"; EXP_HOOK="$SETUP_URL"
  ev "express webhook_url=$EXP_HOOK (relative: the operator must prefix the control plane origin)"
else
  SETUP_OK=0
fi
if [ "$SKIP_MONO" -eq 0 ]; then
  git_clone "$MONO_REPO" "$GIT_DIR_ROOT/mono" || SETUP_OK=0
  if setup_app "$MONO_APP" "$MONO_REPO" 8080 dockerfile services/backend "services/backend/**"; then
    MONO_SECRET="$SETUP_SECRET"
  else
    SETUP_OK=0
  fi
fi
if [ "$SETUP_OK" -ne 1 ]; then
  verdict 0 "setup" FAIL "apps and git sources configured" "setup failed, see stderr"
  jq -s . "$ROWS"; exit 1
fi
if ! deploys_json "$EXP_APP" | jq -e 'any(.[]; .status=="succeeded")' >/dev/null; then
  baseline "$EXP_APP" "$EXP_REPO" main railpack "" || ev "express baseline deploy did not succeed"
fi
BASE_ATTEMPT=$(deploys_json "$EXP_APP" | jq -r '[.[] | select(.status=="succeeded")][0].id // empty')
BASE_IMAGE=$(deploys_json "$EXP_APP" | jq -r '[.[] | select(.status=="succeeded")][0].image // empty')
if [ "$SKIP_MONO" -eq 0 ]; then
  if ! deploys_json "$MONO_APP" | jq -e 'any(.[]; .status=="succeeded")' >/dev/null; then
    baseline "$MONO_APP" "$MONO_REPO" main dockerfile services/backend || ev "monorepo baseline deploy did not succeed"
  fi
fi
verdict 0 "setup" PASS "apps, git source (repo, branch main, trigger push), secret and baseline deploy ready" \
  "baseline attempt $BASE_ATTEMPT image $BASE_IMAGE"

EXP_DIR="$GIT_DIR_ROOT/express"
exp_sed() { printf "s/res\\\\.send\\\\('lr-qa express ok[^']*'\\\\)/res.send('lr-qa express ok %s')/" "$1"; }
FILES='["server.js"]'

EXP_PARENT=$(git -C "$EXP_DIR" rev-parse HEAD)
C1=$(edit_commit "$EXP_DIR" server.js "$(exp_sed "gitflow-$TAG-1")" "test: gitflow $TAG push 1") || C1=""
push_ref "$EXP_DIR" origin main
C2=$(edit_commit "$EXP_DIR" server.js "$(exp_sed "gitflow-$TAG-2")" "test: gitflow $TAG push 2") || C2=""
C3=$(edit_commit "$EXP_DIR" server.js "$(exp_sed "gitflow-$TAG-3")" "test: gitflow $TAG push 3") || C3=""
if [ -z "$C1" ] || [ -z "$C2" ] || [ -z "$C3" ]; then
  echo "could not create fixture commits" >&2; exit 2
fi

# ================================================================ 1 happy path
if selected 1; then
  begin 1 "push to main deploys that exact commit"
  before_count=$(attempt_count "$EXP_APP")
  payload refs/heads/main "$EXP_PARENT" "$C1" "$EXP_REPO" "test: gitflow $TAG push 1" '[]' "$FILES" '[]' >"$TMP/p1.json"
  t0=$(now)
  send "$EXP_APP" "$EXP_SECRET" push "lrqa-$TAG-d1" good "$TMP/p1.json"
  secs=$(($(now) - t0))
  ev "webhook response took ${secs}s (the build runs inside the request; GitHub gives up after 10s)"
  wait_sha "$EXP_APP" "$C1"
  show_attempt "$EXP_APP" "$C1"
  wait_body "$EXP_APP" "gitflow-$TAG-1" 90; body_ok=$?
  ev "app body: $(printf '%s' "$BODY" | head -c 120)"
  row=$(attempts_for "$EXP_APP" "$C1" | jq -c '.[0] // {}')
  src=$(printf '%s' "$row" | jq -r '.source // ""')
  msg=$(printf '%s' "$row" | jq -r '.commit_message // ""')
  auth=$(printf '%s' "$row" | jq -r '.author // ""')
  after_count=$(attempt_count "$EXP_APP")
  img_old=""
  if [ -n "$BASE_IMAGE" ] && command -v docker >/dev/null 2>&1; then
    if docker image inspect "$BASE_IMAGE" >/dev/null 2>&1; then img_old=present; else img_old=missing; fi
  fi
  ev "previous image $BASE_IMAGE: ${img_old:-unchecked}; attempts $before_count -> $after_count"
  ev "deploy-safety targets: $("$CLI" apps deploy-safety targets "$EXP_APP" --json 2>&1 | head -c 400)"
  trig=$(api_get "/api/v1/deployments?app=$EXP_APP&limit=5" | jq -r --arg s "$C1" '(if type=="array" then . else (.deployments // .items // []) end) | map(select(.commit_sha==$s))[0].trigger // ""')
  ev "deploy history trigger label: '$trig' (api /api/v1/deployments)"
  bad=""
  [ "$HC" = "200" ] || bad="$bad http=$HC;"
  [ "$W_STATUS" = "succeeded" ] || bad="$bad attempt=$W_STATUS;"
  [ "$body_ok" -eq 0 ] || bad="$bad new content not served;"
  [ "$src" = "webhook" ] || bad="$bad source='$src';"
  [ "$trig" = "git push" ] || bad="$bad trigger='$trig';"
  [ "$img_old" = "missing" ] && bad="$bad previous image gone;"
  [ -n "$msg" ] || bad="$bad commit_message not recorded;"
  [ -n "$auth" ] || bad="$bad author not recorded;"
  if [ -z "$bad" ]; then
    verdict 1 "happy path" PASS "200, attempt for exact SHA succeeded, new body served, source=webhook, previous image kept" \
      "HTTP $HC after ${secs}s, attempt $W_ATTEMPT $W_STATUS, source=$src, author=$auth"
  else
    verdict 1 "happy path" FAIL "200, attempt for exact SHA succeeded, new body served, source=webhook, previous image kept" "$bad"
  fi
fi

# ================================================================ 2 bad / missing signature
if selected 2; then
  begin 2 "wrong or missing signature is rejected, nothing deployed"
  before_count=$(attempt_count "$EXP_APP")
  payload refs/heads/main "$C1" "$C2" "$EXP_REPO" "test: gitflow $TAG push 2" '[]' "$FILES" '[]' >"$TMP/p2.json"
  send "$EXP_APP" "$EXP_SECRET" push "lrqa-$TAG-bad" bad "$TMP/p2.json"; hc_bad="$HC"
  send "$EXP_APP" "$EXP_SECRET" push "lrqa-$TAG-none" none "$TMP/p2.json"; hc_none="$HC"
  sleep 2
  after_count=$(attempt_count "$EXP_APP")
  n_sha=$(attempts_for "$EXP_APP" "$C2" | jq 'length')
  if [ "$hc_bad" = "401" ] && [ "$hc_none" = "401" ] && [ "$after_count" = "$before_count" ] && [ "$n_sha" = "0" ]; then
    verdict 2 "bad and missing signature" PASS "401 and 401, attempts unchanged" "HTTP $hc_bad / $hc_none, attempts $before_count -> $after_count"
  else
    verdict 2 "bad and missing signature" FAIL "401 and 401, attempts unchanged" "HTTP $hc_bad / $hc_none, attempts $before_count -> $after_count, attempts for sha: $n_sha"
  fi
fi

# ================================================================ 3 second push while a build runs
if selected 3; then
  begin 3 "second push while a build is running"
  git -C "$EXP_DIR" push -q origin main 2>&1 | redact >&2
  payload refs/heads/main "$C1" "$C2" "$EXP_REPO" "test: gitflow $TAG push 2" '[]' "$FILES" '[]' >"$TMP/p2.json"
  payload refs/heads/main "$C2" "$C3" "$EXP_REPO" "test: gitflow $TAG push 3" '[]' "$FILES" '[]' >"$TMP/p3.json"
  sig2=$(hmac "$TMP/p2.json" "$EXP_SECRET")
  (curl -s -m "$WAIT" -o "$TMP/resp2" -w '%{http_code}' -X POST -H "Content-Type: application/json" \
     -H "X-GitHub-Event: push" -H "X-GitHub-Delivery: lrqa-$TAG-d2" -H "X-Hub-Signature-256: sha256=$sig2" \
     --data-binary "@$TMP/p2.json" "$API_URL/api/v1/webhooks/github/$EXP_APP" >"$TMP/hc2") &
  bg=$!
  sleep 0.5
  send "$EXP_APP" "$EXP_SECRET" push "lrqa-$TAG-d3" good "$TMP/p3.json"
  hc3="$HC"; hb3="$HB"
  wait "$bg"
  hc2=$(cat "$TMP/hc2" 2>/dev/null); hb2=$(head -c 200 "$TMP/resp2" | tr '\n' ' ')
  ev "first push (background): HTTP $hc2 $hb2"
  wait_sha "$EXP_APP" "$C2"; s2="$W_STATUS"
  wait_sha "$EXP_APP" "$C3"; s3="$W_STATUS"
  show_attempt "$EXP_APP" "$C2"; show_attempt "$EXP_APP" "$C3"
  wait_body "$EXP_APP" "gitflow-$TAG-3" 120; b3=$?
  ev "final body: $(printf '%s' "$BODY" | head -c 100)"
  how="HTTP $hc3 $hb3"
  if [ "$hc3" = "202" ] && [ "$s3" = "succeeded" ] && [ "$b3" -eq 0 ]; then
    verdict 3 "concurrent push" PASS "second push queued (202), deployed after the first, final content is the newest commit" \
      "$how; first=$s2 second=$s3"
  else
    verdict 3 "concurrent push" FAIL "second push queued (202), deployed after the first, final content is the newest commit" \
      "$how; first=$s2 second=$s3 newest-served=$b3 (0=yes)"
  fi
fi

# ================================================================ 4 duplicate delivery, same SHA
if selected 4; then
  begin 4 "duplicate delivery id and same SHA redelivered"
  git -C "$EXP_DIR" push -q origin main 2>&1 | redact >&2
  payload refs/heads/main "$C2" "$C3" "$EXP_REPO" "test: gitflow $TAG push 3" '[]' "$FILES" '[]' >"$TMP/p4.json"
  n0=$(attempts_for "$EXP_APP" "$C3" | jq 'length')
  send "$EXP_APP" "$EXP_SECRET" push "lrqa-$TAG-d3" good "$TMP/p4.json"; hc_dup="$HC"; hb_dup="$HB"
  n1=$(attempts_for "$EXP_APP" "$C3" | jq 'length')
  send "$EXP_APP" "$EXP_SECRET" push "lrqa-$TAG-d3-new-id" good "$TMP/p4.json"; hc_same="$HC"; hb_same="$HB"
  n2=$(attempts_for "$EXP_APP" "$C3" | jq 'length')
  ev "attempts for ${C3:0:12}: $n0 -> $n1 (same delivery id) -> $n2 (new delivery id)"
  if [ "$n1" = "$n0" ]; then
    verdict 4a "duplicate delivery id" PASS "no second deploy for a repeated X-GitHub-Delivery" "HTTP $hc_dup $hb_dup, attempts $n0 -> $n1"
  else
    verdict 4a "duplicate delivery id" FAIL "no second deploy for a repeated X-GitHub-Delivery" \
      "HTTP $hc_dup $hb_dup, attempts $n0 -> $n1 (no delivery-id dedupe, the same SHA is rebuilt)"
  fi
  if [ "$n2" = "$n1" ]; then
    verdict 4b "same SHA redelivered" PASS "no rebuild of an SHA that is already live" "HTTP $hc_same $hb_same, attempts $n1 -> $n2"
  else
    verdict 4b "same SHA redelivered" FAIL "no rebuild of an SHA that is already live" \
      "HTTP $hc_same $hb_same, attempts $n1 -> $n2 (a SHA already serving is rebuilt and redeployed)"
  fi
fi

# ================================================================ 5 ignored refs
if selected 5; then
  begin 5 "ignored refs: other branch, tag push, unrelated repository"
  n0=$(attempt_count "$EXP_APP")
  head_sha=$(git -C "$EXP_DIR" rev-parse HEAD)
  git -C "$EXP_DIR" checkout -q -B gitflow-test
  OB=$(edit_commit "$EXP_DIR" server.js "$(exp_sed "gitflow-$TAG-branch")" "test: gitflow $TAG branch") || OB=""
  git -C "$EXP_DIR" push -q --force origin gitflow-test 2>&1 | redact >&2
  git -C "$EXP_DIR" checkout -q main
  payload refs/heads/gitflow-test "$head_sha" "$OB" "$EXP_REPO" "test: gitflow $TAG branch" '[]' "$FILES" '[]' >"$TMP/p5a.json"
  send "$EXP_APP" "$EXP_SECRET" push "lrqa-$TAG-branch" good "$TMP/p5a.json"; hc_b="$HC"; hb_b="$HB"
  n1=$(attempt_count "$EXP_APP")
  if [ "$hc_b" = "200" ] && [ "$n1" = "$n0" ]; then
    verdict 5a "push to another branch" PASS "200 ignored, no attempt" "HTTP $hc_b $hb_b"
  else
    verdict 5a "push to another branch" FAIL "200 ignored, no attempt" "HTTP $hc_b $hb_b attempts $n0 -> $n1"
  fi

  tagname="gitflow-$TAG"
  git -C "$EXP_DIR" tag -f "$tagname" "$head_sha" >/dev/null 2>&1
  git -C "$EXP_DIR" push -q --force origin "refs/tags/$tagname" 2>&1 | redact >&2
  payload "refs/tags/$tagname" "0000000000000000000000000000000000000000" "$head_sha" "$EXP_REPO" "tag" '[]' '[]' '[]' >"$TMP/p5b.json"
  send "$EXP_APP" "$EXP_SECRET" push "lrqa-$TAG-tag" good "$TMP/p5b.json"; hc_t="$HC"; hb_t="$HB"
  git -C "$EXP_DIR" push -q origin ":refs/tags/$tagname" 2>&1 | redact >&2
  n2=$(attempt_count "$EXP_APP")
  if [ "$hc_t" = "200" ] && [ "$n2" = "$n1" ]; then
    verdict 5b "tag push (trigger_mode push)" PASS "200 ignored, no attempt" "HTTP $hc_t $hb_t"
  else
    verdict 5b "tag push (trigger_mode push)" FAIL "200 ignored, no attempt" "HTTP $hc_t $hb_t attempts $n1 -> $n2"
  fi

  bogus="deadbeefdeadbeefdeadbeefdeadbeefdeadbeef"
  payload refs/heads/main "$head_sha" "$bogus" "someone-else/other-repo" "unrelated" '[]' '["x.js"]' '[]' >"$TMP/p5c.json"
  send "$EXP_APP" "$EXP_SECRET" push "lrqa-$TAG-unk" good "$TMP/p5c.json"; hc_u="$HC"; hb_u="$HB"
  n3=$(attempt_count "$EXP_APP")
  ev "newest attempt after unrelated-repo push: $(deploys_json "$EXP_APP" | jq -c '.[0] | {id,status,source,commit_sha}')"
  if [ "$hc_u" = "200" ] && [ "$n3" = "$n2" ]; then
    verdict 5c "push event for an unrelated repository" PASS "ignored, no attempt" "HTTP $hc_u $hb_u"
  else
    verdict 5c "push event for an unrelated repository" FAIL \
      "ignored, no attempt (repository.full_name differs from the git source repo)" \
      "HTTP $hc_u $hb_u attempts $n2 -> $n3 (repository.full_name is never compared, the app's repo is cloned at the claimed SHA)"
  fi
fi

# ================================================================ 6 malformed, huge, unknown app, ping
if selected 6; then
  begin 6 "malformed JSON, oversize body, unknown app, ping"
  n0=$(attempt_count "$EXP_APP")
  printf '{"ref": "refs/heads/main", "after": ' >"$TMP/bad.json"
  send "$EXP_APP" "$EXP_SECRET" push "lrqa-$TAG-mal" good "$TMP/bad.json"; hc_m="$HC"
  send "$EXP_APP" "$EXP_SECRET" push "lrqa-$TAG-mal2" none "$TMP/bad.json"; hc_m2="$HC"
  if [ "$hc_m" = "400" ] && [ "$hc_m2" = "401" ]; then
    verdict 6a "malformed JSON" PASS "signed: 400, unsigned: 401" "HTTP $hc_m / $hc_m2"
  else
    verdict 6a "malformed JSON" FAIL "signed: 400, unsigned: 401" "HTTP $hc_m / $hc_m2"
  fi

  head -c 27000000 /dev/zero | tr '\0' 'a' >"$TMP/huge.json"
  send "$EXP_APP" "$EXP_SECRET" push "lrqa-$TAG-huge" good "$TMP/huge.json"; hc_h="$HC"
  rm -f "$TMP/huge.json"
  if [ "$hc_h" = "400" ] || [ "$hc_h" = "413" ]; then
    verdict 6b "27 MB payload" PASS "rejected (400/413) without deploying" "HTTP $hc_h"
  else
    verdict 6b "27 MB payload" FAIL "rejected (400/413) without deploying" "HTTP $hc_h"
  fi

  payload refs/heads/main "$C1" "$C2" "$EXP_REPO" "x" '[]' '[]' '[]' >"$TMP/p6.json"
  send "qa-gitflow-does-not-exist" "$EXP_SECRET" push "lrqa-$TAG-noapp" good "$TMP/p6.json"; hc_n="$HC"
  if [ "$hc_n" = "404" ]; then
    verdict 6c "unknown app" PASS "404" "HTTP $hc_n"
  else
    verdict 6c "unknown app" FAIL "404" "HTTP $hc_n"
  fi

  printf '{"zen":"Keep it logically awesome.","hook_id":1}' >"$TMP/ping.json"
  send "$EXP_APP" "$EXP_SECRET" ping "lrqa-$TAG-ping" good "$TMP/ping.json"; hc_p="$HC"
  n1=$(attempt_count "$EXP_APP")
  if [ "$hc_p" = "200" ] && [ "$n1" = "$n0" ]; then
    verdict 6d "ping event" PASS "200, no deploy" "HTTP $hc_p $HB"
  else
    verdict 6d "ping event" FAIL "200, no deploy" "HTTP $hc_p attempts $n0 -> $n1"
  fi
fi

# ================================================================ 7 pull_request event
if selected 7; then
  begin 7 "pull_request event with previews on, then closed"
  head_sha=$(git -C "$EXP_DIR" rev-parse HEAD)
  git -C "$EXP_DIR" fetch -q origin gitflow-test 2>&1 | redact >&2
  pr_sha=$(git -C "$EXP_DIR" rev-parse FETCH_HEAD 2>/dev/null || echo "$head_sha")
  pr_payload() { # action
    jq -nc --arg a "$1" --arg sha "$pr_sha" --arg base "$head_sha" --arg full "$EXP_REPO" '
      {action:$a, number:9001, pull_request:{number:9001, title:"gitflow test", state:(if $a=="closed" then "closed" else "open" end), draft:false,
        merged:false, head:{ref:"gitflow-test", sha:$sha, repo:{full_name:$full, fork:false}},
        base:{ref:"main", sha:$base, repo:{full_name:$full}}, user:{login:"lr-qa-gitflow"}},
       repository:{full_name:$full, clone_url:("https://github.com/"+$full+".git")}}'
  }
  n0=$(attempt_count "$EXP_APP")
  pr_payload opened >"$TMP/pr.json"
  send "$EXP_APP" "$EXP_SECRET" pull_request "lrqa-$TAG-pr-off" good "$TMP/pr.json"
  hc_off="$HC"; hb_off="$HB"; n1=$(attempt_count "$EXP_APP")
  cli apps previews enable "$EXP_APP" --json; ev "previews enable: rc=$RC $(printf '%s' "$OUT" | head -c 160)"
  send "$EXP_APP" "$EXP_SECRET" pull_request "lrqa-$TAG-pr-on" good "$TMP/pr.json"
  hc_on="$HC"; hb_on="$HB"
  ev "preview list: $("$CLI" apps previews list "$EXP_APP" --json 2>&1 | head -c 500)"
  pr_payload closed >"$TMP/prc.json"
  send "$EXP_APP" "$EXP_SECRET" pull_request "lrqa-$TAG-pr-close" good "$TMP/prc.json"
  hc_close="$HC"
  cli apps previews disable "$EXP_APP" --json
  if [ "$hc_off" = "200" ] && [ "$n1" = "$n0" ] && [ "$hc_on" -lt 300 ] && [ "$hc_close" -lt 300 ]; then
    verdict 7 "pull_request preview" PASS "previews off: ignored, production untouched; on: opened accepted; closed accepted" \
      "off HTTP $hc_off $hb_off; on HTTP $hc_on $hb_on; closed HTTP $hc_close"
  else
    verdict 7 "pull_request preview" FAIL "previews off: ignored, production untouched; on: opened accepted; closed accepted" \
      "off HTTP $hc_off $hb_off (attempts $n0 -> $n1); on HTTP $hc_on $hb_on; closed HTTP $hc_close"
  fi
fi

# ================================================================ 8 force push
if selected 8; then
  begin 8 "force push: after SHA is not a descendant of the deployed commit"
  head_sha=$(git -C "$EXP_DIR" rev-parse HEAD)
  git -C "$EXP_DIR" checkout -q -B gitflow-force "$EXP_PARENT"
  FP=$(edit_commit "$EXP_DIR" server.js "$(exp_sed "gitflow-$TAG-forced")" "test: gitflow $TAG forced rewrite") || FP=""
  git -C "$EXP_DIR" push -q --force origin gitflow-force 2>&1 | redact >&2
  git -C "$EXP_DIR" checkout -q main
  payload refs/heads/main "$head_sha" "$FP" "$EXP_REPO" "test: gitflow $TAG forced rewrite" '[]' "$FILES" '[]' true >"$TMP/p8.json"
  send "$EXP_APP" "$EXP_SECRET" push "lrqa-$TAG-force" good "$TMP/p8.json"
  wait_sha "$EXP_APP" "$FP"
  show_attempt "$EXP_APP" "$FP"
  if [ "$HC" = "200" ] && [ "$W_STATUS" = "succeeded" ]; then
    verdict 8 "force push to main" PASS "rewritten tip deploys (200), it is the new truth for the branch" "HTTP $HC, attempt $W_STATUS"
  else
    verdict 8 "force push to main" FAIL "rewritten tip deploys (200), it is the new truth for the branch" "HTTP $HC $HB attempt=$W_STATUS"
  fi
fi

# ================================================================ 9 path filter on the monorepo
if selected 9 && [ "$SKIP_MONO" -eq 0 ]; then
  begin 9 "deploy_paths services/backend/** on the monorepo"
  MD="$GIT_DIR_ROOT/mono"
  mparent=$(git -C "$MD" rev-parse HEAD)
  n0=$(attempt_count "$MONO_APP")
  FE=$(edit_commit "$MD" services/frontend/index.html "s#<p>Static frontend service[^<]*</p>#<p>Static frontend service gitflow-$TAG-fe</p>#" "test: gitflow $TAG frontend only") || FE=""
  push_ref "$MD" origin main
  payload refs/heads/main "$mparent" "$FE" "$MONO_REPO" "test: gitflow $TAG frontend only" '[]' '["services/frontend/index.html"]' '[]' >"$TMP/p9a.json"
  send "$MONO_APP" "$MONO_SECRET" push "lrqa-$TAG-fe" good "$TMP/p9a.json"; hc_fe="$HC"; hb_fe="$HB"
  n1=$(attempt_count "$MONO_APP")
  if [ "$hc_fe" = "200" ] && [ "$n1" = "$n0" ]; then
    verdict 9a "frontend-only commit" PASS "200 ignored by path filter, no attempt" "HTTP $hc_fe $hb_fe"
  else
    verdict 9a "frontend-only commit" FAIL "200 ignored by path filter, no attempt" "HTTP $hc_fe $hb_fe attempts $n0 -> $n1"
  fi
  BE=$(edit_commit "$MD" services/backend/server.js "s#'lr-qa monorepo-paths backend ok[^']*'#'lr-qa monorepo-paths backend ok gitflow-$TAG-be'#" "test: gitflow $TAG backend") || BE=""
  push_ref "$MD" origin main
  payload refs/heads/main "$FE" "$BE" "$MONO_REPO" "test: gitflow $TAG backend" '[]' '["services/backend/server.js"]' '[]' >"$TMP/p9b.json"
  send "$MONO_APP" "$MONO_SECRET" push "lrqa-$TAG-be" good "$TMP/p9b.json"; hc_be="$HC"; hb_be="$HB"
  wait_sha "$MONO_APP" "$BE"
  show_attempt "$MONO_APP" "$BE"
  wait_body "$MONO_APP" "gitflow-$TAG-be" 90; mb=$?
  if [ "$hc_be" = "200" ] && [ "$W_STATUS" = "succeeded" ] && [ "$mb" -eq 0 ]; then
    verdict 9b "backend commit" PASS "deploys and serves the new content" "HTTP $hc_be, attempt $W_STATUS, body ok"
  else
    verdict 9b "backend commit" FAIL "deploys and serves the new content" "HTTP $hc_be $hb_be attempt=$W_STATUS body=$mb (0=ok)"
  fi
fi

# ================================================================ 10 delivery log
if selected 10; then
  begin 10 "delivery log records ignored and rejected deliveries"
  dl=$(api_get "/api/v1/apps/$EXP_APP/webhook-deliveries?limit=100")
  ev "raw head: $(printf '%s' "$dl" | head -c 300)"
  ev "deliveries [status,event,matched,sigvalid,note]: $(printf '%s' "$dl" | jq -c '[(if type=="array" then . else (.deliveries // []) end)[]? | [.status_code,.event_type,.matched,.signature_valid,((.note // .error // "")[0:60])]] | .[0:14]' 2>/dev/null | head -c 1200)"
  total=$(printf '%s' "$dl" | jq '(if type=="array" then . else (.deliveries // []) end) | length' 2>/dev/null)
  rej=$(printf '%s' "$dl" | jq '[(if type=="array" then . else (.deliveries // []) end)[]? | select(.status_code==401)] | length' 2>/dev/null)
  ign=$(printf '%s' "$dl" | jq '[(if type=="array" then . else (.deliveries // []) end)[]? | select(.status_code==200 and ((.note // "") | startswith("ignored")))] | length' 2>/dev/null)
  if [ "${total:-0}" -gt 0 ] && [ "${rej:-0}" -gt 0 ] && [ "${ign:-0}" -gt 0 ]; then
    verdict 10 "delivery log" PASS "401s and ignored pushes are listed with a reason" "$total deliveries, $rej rejected, $ign ignored"
  else
    verdict 10 "delivery log" FAIL "401s and ignored pushes are listed with a reason" "total=${total:-?} rejected=${rej:-?} ignored=${ign:-?}"
  fi
fi

# ================================================================ report
mkdir -p "$OUT_DIR"
JSON="$OUT_DIR/gitflow-$TS.json"; MDF="$OUT_DIR/gitflow-$TS.md"
VERSION=$(api_get /api/v1/version 2>/dev/null | jq -r '.version // empty' 2>/dev/null)
jq -s --arg run "$RUN_ID" --arg api "$API_URL" --arg ver "$VERSION" --arg tag "$TAG" \
  '{run_id:$run, api:$api, version:$ver, marker:$tag, results:.}' "$ROWS" >"$JSON"
{
  echo "# Git push-to-deploy conformance $RUN_ID"
  echo
  echo "Plane: $API_URL ${VERSION:+(version $VERSION)}. Deliveries are GitHub-format push events replayed with a valid HMAC."
  echo
  echo "| # | Scenario | Verdict | Expected | Actual |"
  echo "|---|---|---|---|---|"
  jq -r '.results[] | "| \(.id) | \(.scenario) | \(.verdict) | \(.expected | gsub("\\|";"/")) | \(.actual | gsub("\\|";"/")) |"' "$JSON"
  echo
  echo "## Evidence"
  jq -r '.results[] | "### \(.id) \(.scenario): \(.verdict)\n\n```\n\(.evidence)\n```\n"' "$JSON"
  echo "## Apps"
  echo
  echo "| App | Fallback URL | Host port |"
  echo "|---|---|---|"
  for a in "$EXP_APP" "$MONO_APP"; do
    app_exists "$a" || continue
    fb=$("$CLI" apps get "$a" --json 2>/dev/null | jq -r '.fallback_url // "-"')
    echo "| $a | $fb | $(host_port "$a") |"
  done
} >"$MDF"
echo "wrote $JSON and $MDF" >&2
jq -r '.results[] | "\(.verdict)\t\(.id)\t\(.scenario)\t\(.actual)"' "$JSON"
if jq -e 'any(.results[]; .verdict=="FAIL")' "$JSON" >/dev/null; then exit 1; fi
exit 0
