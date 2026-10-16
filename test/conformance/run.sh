#!/usr/bin/env bash
# Deploy conformance runner: imports, deploys, probes, redeploys, rolls back and
# deletes each fixture in fixtures.json through the real CLI. bash 3.2 safe.
# Usage: APP_PROFILE=<profile> test/conformance/run.sh [--only a,b] [--wave N]
#   [--jobs N | --load] [--keep] [--timeout 900] [--host-header] [--i-know]
#   [--resume RUN] [--failed-from RUN] [--run-id RUN] [--report RUN] [--selftest]
set -u

HERE=$(cd "$(dirname "$0")" && pwd)
REPO_ROOT=$(cd "$HERE/../.." && pwd)
FIXTURES="$HERE/fixtures.json"
ALLOWED="$HERE/allowed-hosts"
OUT_DIR="$HERE/results"

ONLY=""; KEEP=0; TIMEOUT=900; HOST_HEADER=0; I_KNOW=0; SELFTEST=0
JOBS=4; LOAD=0; WAVE=""; RESUME=""; FAILED_FROM=""; RUN_ID=""
WORKER=""; QUEUED_AT=0; REPORT=""

while [ $# -gt 0 ]; do
  case "$1" in
    --only) ONLY="${2:-}"; shift 2 ;;
    --keep) KEEP=1; shift ;;
    --timeout) TIMEOUT="${2:-900}"; shift 2 ;;
    --host-header) HOST_HEADER=1; shift ;;
    --i-know) I_KNOW=1; shift ;;
    --selftest) SELFTEST=1; shift ;;
    --jobs) JOBS="${2:-4}"; shift 2 ;;
    --load) LOAD=1; shift ;;
    --wave) WAVE="${2:-}"; shift 2 ;;
    --resume) RESUME="${2:-}"; shift 2 ;;
    --failed-from) FAILED_FROM="${2:-}"; shift 2 ;;
    --run-id) RUN_ID="${2:-}"; shift 2 ;;
    --report) REPORT="${2:-}"; shift 2 ;;
    --worker) WORKER="${2:-}"; shift 2 ;;
    --queued-at) QUEUED_AT="${2:-0}"; shift 2 ;;
    -h|--help) sed -n 2,6p "$0" | sed 's/^# \{0,1\}//'; exit 0 ;;
    *) echo "unknown flag: $1" >&2; exit 2 ;;
  esac
done

for tool in jq curl; do
  command -v "$tool" >/dev/null 2>&1 || { echo "missing tool: $tool" >&2; exit 2; }
done

TMP=$(mktemp -d "${TMPDIR:-/tmp}/lr-conformance.XXXXXX")
trap 'rm -rf "$TMP"' EXIT

# One CLI build for the whole run; workers inherit it through LEVELRAIL_CLI.
if [ -n "${LEVELRAIL_CLI:-}" ]; then
  CLI="$LEVELRAIL_CLI"
else
  CLI="$TMP/levelrail-cli"
  echo "building CLI..." >&2
  (cd "$REPO_ROOT" && go build -o "$CLI" ./cmd/levelrail-cli) || { echo "CLI build failed" >&2; exit 2; }
  export LEVELRAIL_CLI="$CLI"
fi

cred_value() { # key
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
API_HOST=$(printf '%s' "$API_URL" | sed -e 's#^[a-zA-Z]*://##' -e 's#[/?].*$##' -e 's#^.*@##' -e 's#:[0-9]*$##')

host_allowed() {
  grep -v '^[[:space:]]*#' "$ALLOWED" 2>/dev/null | tr -d '[:blank:]' | grep -qxF -- "$1"
}
if ! host_allowed "$API_HOST"; then
  if [ "$I_KNOW" -ne 1 ]; then
    echo "refusing: API host '$API_HOST' is not in $ALLOWED (pass --i-know to override)" >&2
    exit 3
  fi
  [ -n "$WORKER" ] || echo "warning: running against non-allowlisted host $API_HOST (--i-know)" >&2
fi
case "$API_HOST" in localhost|127.0.0.1) LOCAL_API=1 ;; *) LOCAL_API=0 ;; esac

lr() { "$CLI" "$@"; }
now() { date +%s; }

if [ "$SELFTEST" -eq 1 ]; then FX_KEY=selftest; else FX_KEY=fixtures; fi
fx() { jq -r --arg n "$1" ".${FX_KEY}[] | select(.name==\$n) | $2" "$FIXTURES"; }

# ---------------------------------------------------------------- progress
RUN_DIR=""
STATE_DIR=""
write_progress() {
  local tmp f n
  tmp="$OUT_DIR/progress.txt.$$"
  {
    echo "run $(basename "$RUN_DIR") updated $(date +%H:%M:%S) host $API_HOST"
    for f in "$STATE_DIR"/*; do
      [ -f "$f" ] || continue
      n=$(basename "$f")
      printf '%-28s %s\n' "$n" "$(cat "$f")"
    done
  } >"$tmp"
  mv -f "$tmp" "$OUT_DIR/progress.txt"
}
set_state() { # name state
  printf '%s since %s' "$2" "$(date +%H:%M:%S)" >"$STATE_DIR/$1"
  write_progress
}

# ---------------------------------------------------------------- worker state
F_DETECT=false; F_BUILD=false; F_RUN=false; F_HTTP=false; F_HEALTH=false
F_REDEPLOY=null; F_ROLLBACK=null
F_STAGE=""; F_EXCERPT=""; F_DET_FW=""; F_DET_PORT=""; F_DET_BUILD=""; F_DET_NOTE=""
F_BUILD_SECS=0; F_CONCURRENT=0; F_ROLLBACK_NOTE=""; F_HTTP_NOTE=""
DEADLINE=0
W_STATUS=""; W_JSON=""
H_CODE=""; H_URL=""; H_HH=""
A_JSON=""
CREATED_APPS=""
IS_STATIC=0
FAIL_STEP=""; FAIL_TEXT=""

fail() {
  if [ -z "$F_STAGE" ]; then
    F_STAGE="$1"
    F_EXCERPT=$(printf '%s' "${2:-}" | tail -c 1500)
  else
    F_EXCERPT=$(printf '%s\n[then %s] %s' "$F_EXCERPT" "$1" "${2:-}" | tail -c 2500)
  fi
}
past_deadline() { [ "$(now)" -ge "$DEADLINE" ]; }
app_exists() { lr apps get "$1" --json >/dev/null 2>&1; }
newest_id() { lr apps deploys list "$1" --json 2>/dev/null | jq -r '.[0].id // empty'; }

# Poll one deploy to a terminal status. Sets W_STATUS and W_JSON (deploy show).
# "apps deploys wait" is not used: it never settles for container-less static deploys.
wait_deploy() {
  local app="$1" id="$2" out st
  W_STATUS="timeout"; W_JSON=""
  while ! past_deadline; do
    [ -n "$id" ] || id=$(newest_id "$app")
    if [ -n "$id" ]; then
      out=$(lr apps deploys show "$app" "$id" --json 2>/dev/null)
      st=$(printf '%s' "$out" | jq -r '.status // empty' 2>/dev/null)
      W_JSON="$out"
      case "$st" in
        succeeded|failed|canceled|cancelled|superseded|blocked) W_STATUS="$st"; return 0 ;;
      esac
    fi
    sleep 3
  done
  return 1
}

# Wait for a deploy newer than prev (rollback-to does not print its id).
wait_new_deploy() {
  local app="$1" prev="$2" id=""
  W_STATUS="timeout"
  while ! past_deadline; do
    id=$(newest_id "$app")
    if [ -n "$id" ] && [ "$id" != "$prev" ]; then wait_deploy "$app" "$id"; return; fi
    sleep 2
  done
  return 1
}

failure_detail() {
  local app="$1" id="$2" cause excerpt logs
  FAIL_STEP=$(printf '%s' "$W_JSON" | jq -r '.failure.failing_step // empty' 2>/dev/null)
  cause=$(printf '%s' "$W_JSON" | jq -r '.failure.cause // empty' 2>/dev/null)
  excerpt=$(printf '%s' "$W_JSON" | jq -r '.failure.log_excerpt // empty' 2>/dev/null)
  [ -n "$id" ] || id=$(newest_id "$app")
  logs=""
  [ -n "$id" ] && logs=$(lr apps deploys logs "$app" "$id" 2>&1 | tail -n 40)
  FAIL_TEXT="${cause}
${excerpt}
${logs}"
}

http_probe() { # url path hostheader
  if [ -n "$3" ]; then
    curl -sk -m 10 -H "Host: $3" -o "$TMP/body" -w '%{http_code}' "${1}${2}" 2>/dev/null
  else
    curl -sk -m 10 -o "$TMP/body" -w '%{http_code}' "${1}${2}" 2>/dev/null
  fi
}

# Candidate base URLs, one per line as "url|hostheader".
base_urls() {
  local app="$1" net host_port fallback pub dom
  net=$(lr apps network "$app" --json 2>/dev/null)
  host_port=$(printf '%s' "$net" | jq -r '.host_port // empty')
  fallback=$(printf '%s' "$net" | jq -r '.fallback_url // empty')
  pub=$(printf '%s' "$net" | jq -r '.public_host // empty')
  if [ "$HOST_HEADER" -eq 1 ]; then
    dom=$(lr apps get "$app" --json 2>/dev/null | jq -r '.domains[0] // empty')
    [ -z "$dom" ] && dom=$(printf '%s' "$fallback" | sed -e 's#^[a-zA-Z]*://##' -e 's#/.*$##')
    echo "http://127.0.0.1:${INGRESS_PORT:-80}|$dom"
    return
  fi
  if [ "$LOCAL_API" -eq 1 ] && [ -n "$host_port" ]; then echo "http://127.0.0.1:${host_port}|"; fi
  if [ -n "$fallback" ]; then echo "${fallback}|"; fi
  if [ -n "$host_port" ] && [ "$LOCAL_API" -eq 0 ]; then echo "http://${pub:-$API_HOST}:${host_port}|"; fi
  return 0
}

probe_root() { # app want
  local tries=0 line url hh code
  while [ $tries -lt 45 ] && ! past_deadline; do
    for line in $(base_urls "$1"); do
      url=${line%%|*}; hh=${line#*|}
      code=$(http_probe "$url" "/" "$hh")
      H_CODE="$code"; H_URL="$url"
      if [ "$code" = "200" ] && grep -qF -- "$2" "$TMP/body" 2>/dev/null; then
        H_HH="$hh"
        return 0
      fi
    done
    tries=$((tries + 1))
    sleep 4
  done
  return 1
}

network_running() {
  local i=0
  while [ $i -lt 20 ] && ! past_deadline; do
    [ "$(lr apps network "$1" --json 2>/dev/null | jq -r '.running // false')" = "true" ] && return 0
    i=$((i + 1)); sleep 3
  done
  return 1
}

http_checks() { # app want healthpath
  local code
  if probe_root "$1" "$2"; then
    F_HTTP=true
  elif [ "$LOCAL_API" -eq 1 ] && [ "$IS_STATIC" -eq 1 ] && [ "$H_CODE" = "000" ]; then
    F_HTTP=null; F_HEALTH=null
    F_HTTP_NOTE="static site is served over the ingress with ACME TLS, not reachable from a local plane"
    return 0
  else
    F_HTTP=false
    fail http "GET / answered ${H_CODE:-none} at ${H_URL:-no url}, expected 200 containing: $2
$(head -c 400 "$TMP/body" 2>/dev/null)"
    return 1
  fi
  code=$(http_probe "$H_URL" "$3" "$H_HH")
  case "$code" in
    2??) F_HEALTH=true ;;
    *) F_HEALTH=false; fail health "GET $3 answered $code at $H_URL" ;;
  esac
}

# With GIT_TOKEN the analysis goes straight to the API: the CLI has no
# one-shot token flag for import analysis.
analyze_repo() { # url ref root
  local body tok
  if [ -n "${GIT_TOKEN:-}" ]; then
    tok="${APP_API_TOKEN:-$(cred_value APP_API_TOKEN)}"
    body=$(jq -n --arg u "$1" --arg r "$2" --arg d "$3" --arg t "$GIT_TOKEN" \
      '{repo_url:$u,ref:$r,base_directory:$d,token:$t}')
    A_JSON=$(curl -s -m 60 -X POST "${API_URL%/}/api/v1/imports/analyze" \
      -H "Authorization: Bearer $tok" -H 'Content-Type: application/json' -d "$body")
    return 0
  fi
  local a=(import "$1" --analyze --json)
  [ -n "$2" ] && a+=(--ref "$2")
  [ -n "$3" ] && a+=(--base-directory "$3")
  A_JSON=$(lr "${a[@]}" 2>&1)
}

run_fixture() {
  local name="$1" app repo image root exp_port exp_kind exp_health want url
  local id btype branch dockerfile ok rc out hv ids prev is_static bstart alt_ref fx_branch
  local -a args
  F_DETECT=false; F_BUILD=false; F_RUN=false; F_HTTP=false; F_HEALTH=false
  F_REDEPLOY=null; F_ROLLBACK=null; F_STAGE=""; F_EXCERPT=""; F_ROLLBACK_NOTE=""; F_HTTP_NOTE=""
  F_DET_FW=""; F_DET_PORT=""; F_DET_BUILD=""; F_DET_NOTE=""
  btype=""; branch=""; dockerfile=""; url=""; out=""; rc=0; id=""; hv=""; ids=""; is_static=0
  DEADLINE=$(($(now) + TIMEOUT))
  app="qa-$name"
  repo=$(fx "$name" '.repo // empty')
  image=$(fx "$name" '.image // empty')
  root=$(fx "$name" '.root_dir // empty')
  exp_port=$(fx "$name" '.expect.port')
  exp_kind=$(fx "$name" '.expect.kind')
  exp_health=$(fx "$name" '.expect.health_path // "/"')
  want=$(fx "$name" '.expect.body_contains // empty')
  alt_ref=$(fx "$name" '.alt_ref // empty')
  [ -n "$want" ] || want="lr-qa $(printf '%s' "${repo##*/}" | sed 's/^lr-qa-//') ok"
  echo "==> $app" >&2

  case "$app" in qa-*) ;; *) fail guard "refusing non qa-* app name"; return ;; esac
  if app_exists "$app"; then
    fail preexisting "app $app already exists and was not created by this run; left untouched"
    return
  fi

  # detect. Port and build kind decide; the framework label is recorded as data.
  if [ -n "$image" ]; then
    out=$(lr import "$image" --name "$app" --port "$exp_port" --json 2>&1)
    F_DET_BUILD=$(printf '%s' "$out" | jq -r '.services[0].build // empty' 2>/dev/null)
    F_DET_FW="image"; F_DET_PORT="$exp_port"
    if [ "$F_DET_BUILD" = "image" ]; then F_DETECT=true; else fail detect "$out"; fi
  else
    url="https://github.com/${repo}"
    analyze_repo "$url" "" "$root"
    if ! printf '%s' "$A_JSON" | jq -e '.stack' >/dev/null 2>&1; then
      fail detect "analysis failed: $A_JSON"
    else
      F_DET_FW=$(printf '%s' "$A_JSON" | jq -r '[.stack.framework, .stack.runtime] | map(select(. != null and . != "")) | join(" / ")')
      F_DET_PORT=$(printf '%s' "$A_JSON" | jq -r '.port.value // 0')
      F_DET_BUILD=$(printf '%s' "$A_JSON" | jq -r '.build.type // empty')
      btype="$F_DET_BUILD"
      branch=$(printf '%s' "$A_JSON" | jq -r '.branch // empty')
      fx_branch=$(fx "$name" '.branch // empty')
      [ -n "$fx_branch" ] && branch="$fx_branch"
      dockerfile=$(printf '%s' "$A_JSON" | jq -r '.build.dockerfile_path // empty')
      ok=true
      if [ "$F_DET_PORT" != "$exp_port" ]; then
        if [ "$exp_kind" = "compose" ]; then
          F_DET_NOTE="port $F_DET_PORT != $exp_port (compose, note only)"
        else
          ok=false; F_DET_NOTE="port $F_DET_PORT != $exp_port"
        fi
      fi
      case "$exp_kind" in
        static) [ "$btype" = "static" ] || { ok=false; F_DET_NOTE="$F_DET_NOTE build '$btype' not static"; } ;;
        compose) case "$btype" in compose|dockerfile) ;; *) ok=false; F_DET_NOTE="$F_DET_NOTE build '$btype' not compose" ;; esac ;;
        *) [ "$btype" != "static" ] || { ok=false; F_DET_NOTE="$F_DET_NOTE build detected static"; } ;;
      esac
      hv=$(printf '%s' "$A_JSON" | jq -r '.health.value // empty')
      [ -z "$hv" ] || [ "$hv" = "$exp_health" ] || F_DET_NOTE="$F_DET_NOTE health '$hv' != $exp_health (note)"
      if [ "$ok" = true ]; then F_DETECT=true; else fail detect "$F_DET_NOTE"; fi
    fi
  fi
  [ "$btype" = "static" ] && is_static=1
  [ "$exp_kind" = "static" ] && is_static=1
  IS_STATIC=$is_static

  # deploy. A detection mismatch does not stop the attempt.
  set_state "$name" building
  bstart=$(now)
  mkdir -p "$RUN_DIR/building"
  : >"$RUN_DIR/building/$name"
  F_CONCURRENT=$(find "$RUN_DIR/building" -type f | wc -l | tr -d ' ')
  if [ -n "$image" ]; then
    out=$(lr import "$image" --name "$app" --port "$exp_port" --deploy --json 2>&1); rc=$?
    if [ $rc -ne 0 ]; then fail deploy "$out"; return; fi
    CREATED_APPS="$CREATED_APPS $app"
  elif [ -n "${GIT_TOKEN:-}" ]; then
    [ -n "$btype" ] || btype=railpack
    out=$(lr apps create --name "$app" --image "$app:pending" --port "${F_DET_PORT:-$exp_port}" --json 2>&1); rc=$?
    if [ $rc -ne 0 ]; then fail deploy "create: $out"; return; fi
    CREATED_APPS="$CREATED_APPS $app"
    args=(apps git-source set "$app" --repo-url "$url" --build-type "$btype" --token-secret "$GIT_TOKEN" --json)
    [ -n "$branch" ] && args+=(--branch "$branch")
    [ -n "$root" ] && args+=(--base-directory "$root")
    [ -n "$dockerfile" ] && args+=(--dockerfile "$dockerfile")
    out=$(lr "${args[@]}" 2>&1) || { fail deploy "git-source: $out"; return; }
    args=(apps builds trigger "$app" --repo "$url" --ref "${branch:-main}" --image-repo "$app" --build-type "$btype" --json)
    [ -n "$root" ] && args+=(--base-directory "$root")
    [ -n "$dockerfile" ] && args+=(--dockerfile "$dockerfile")
    out=$(lr "${args[@]}" 2>&1) || { fail deploy "build trigger: $out"; return; }
    id=$(printf '%s' "$out" | jq -r '.id // .deploy_attempt_id // empty' 2>/dev/null)
  else
    args=(import "$url" --apply --name "$app" --no-database --json)
    [ -n "$root" ] && args+=(--base-directory "$root")
    out=$(lr "${args[@]}" 2>&1); rc=$?
    app_exists "$app" && CREATED_APPS="$CREATED_APPS $app"
    if [ $rc -ne 0 ]; then fail deploy "$out"; return; fi
    id=$(printf '%s' "$out" | jq -r '.deploy_attempt_id // empty' 2>/dev/null)
  fi

  wait_deploy "$app" "$id"
  F_BUILD_SECS=$(($(now) - bstart))
  rm -f "$RUN_DIR/building/$name"
  [ -n "$id" ] || id=$(newest_id "$app")
  if [ "$W_STATUS" = "succeeded" ]; then
    F_BUILD=true
    if [ "$is_static" -eq 1 ] || network_running "$app"; then
      F_RUN=true
    else
      fail run "deploy succeeded but apps network reports not running"
    fi
  else
    failure_detail "$app" "$id"
    case "$FAIL_TEXT" in
      *"supported yet"*) F_STAGE=""; fail unsupported "$(printf '%s' "$FAIL_TEXT" | grep -m1 'supported yet')"; return ;;
    esac
    case "$FAIL_STEP" in
      *run*|*ready*|*health*|*cutover*|*start*) F_BUILD=true; fail run "status=$W_STATUS step=$FAIL_STEP $FAIL_TEXT" ;;
      *) fail build "status=$W_STATUS step=$FAIL_STEP $FAIL_TEXT" ;;
    esac
    [ "$W_STATUS" = "timeout" ] && F_STAGE=timeout
    return
  fi

  set_state "$name" checking
  http_checks "$app" "$want" "$exp_health"

  # redeploy with no change
  if [ "$F_RUN" = true ]; then
    if [ -n "$image" ]; then
      out=$(lr apps restart "$app" --json 2>&1); rc=$?
      sleep 3
    else
      prev=$(newest_id "$app")
      args=(apps builds trigger "$app" --repo "$url" --ref "${branch:-main}" --image-repo "$app" --build-type "${btype:-railpack}" --json)
      [ -n "$root" ] && args+=(--base-directory "$root")
      [ -n "$dockerfile" ] && args+=(--dockerfile "$dockerfile")
      out=$(lr "${args[@]}" 2>&1); rc=$?
      if [ $rc -eq 0 ]; then
        wait_new_deploy "$app" "$prev"
        [ "$W_STATUS" = "succeeded" ] || rc=1
      fi
    fi
    if [ $rc -eq 0 ] && { [ "$F_HTTP" = null ] || probe_root "$app" "$want"; }; then
      F_REDEPLOY=true
    else
      F_REDEPLOY=false
      fail redeploy "redeploy rc=$rc status=$W_STATUS: $out"
    fi
  fi

  # rollback: with alt_ref, deploy a distinct commit first so the target is real
  if [ "$F_RUN" = true ]; then
    ids=$(lr apps deploys list "$app" --json 2>/dev/null | jq -r '[.[] | select(.status=="succeeded")] | .[0].id // empty')
    if [ -n "$alt_ref" ] && [ -z "$image" ] && [ -n "$ids" ]; then
      prev=$(newest_id "$app")
      args=(apps builds trigger "$app" --repo "$url" --ref "$alt_ref" --image-repo "$app" --build-type "${btype:-railpack}" --json)
      [ -n "$root" ] && args+=(--base-directory "$root")
      lr "${args[@]}" >/dev/null 2>&1
      wait_new_deploy "$app" "$prev"
    else
      ids=$(lr apps deploys list "$app" --json 2>/dev/null | jq -r '[.[] | select(.status=="succeeded")] | .[1].id // empty')
    fi
    if [ -n "$ids" ]; then
      prev=$(newest_id "$app")
      out=$(lr apps deploys rollback-to "$app" "$ids" --confirm --json 2>&1); rc=$?
      if [ $rc -eq 0 ]; then
        wait_new_deploy "$app" "$prev"
        if [ "$W_STATUS" = "succeeded" ] && { [ "$F_HTTP" = null ] || probe_root "$app" "$want"; }; then
          F_ROLLBACK=true
        else
          F_ROLLBACK=false; fail rollback "status=$W_STATUS after rollback-to $ids"
        fi
      elif printf '%s' "$out" | grep -q "no recorded image digest"; then
        F_ROLLBACK_NOTE="platform gap: static deploys record no image digest, rollback-to returns 409"
      elif printf '%s' "$out" | grep -q "now points at different content"; then
        F_ROLLBACK_NOTE="rollback refused: a no-op rebuild moved the image tag (use alt_ref for a distinct deploy)"
      else
        F_ROLLBACK=false; fail rollback "$out"
      fi
    fi
  fi
}

delete_created() {
  local app out
  for app in $CREATED_APPS; do
    case "$app" in qa-*) ;; *) continue ;; esac
    out=$(lr apps delete "$app" --delete-data --confirm "$app" --json 2>&1) || echo "delete $app failed: $out" >&2
    if app_exists "$app"; then echo "warning: $app still present after delete" >&2; fi
  done
}

# ---------------------------------------------------------------- worker mode
if [ -n "$WORKER" ]; then
  RUN_DIR="$OUT_DIR/$RUN_ID"
  STATE_DIR="$RUN_DIR/state"
  s=$(now)
  qwait=$((s - QUEUED_AT))
  [ "$QUEUED_AT" -gt 0 ] || qwait=0
  run_fixture "$WORKER"
  rm -f "$RUN_DIR/building/$WORKER"
  if [ "$KEEP" -eq 0 ]; then delete_created; fi
  secs=$(($(now) - s))
  if [ -z "$F_STAGE" ]; then final=passed; elif [ "$F_STAGE" = unsupported ]; then final=unsupported; else final=failed; fi
  jq -nc --arg name "$WORKER" --arg app "qa-$WORKER" --arg stage "$F_STAGE" --arg ex "$F_EXCERPT" \
    --arg fw "$F_DET_FW" --arg port "$F_DET_PORT" --arg bt "$F_DET_BUILD" --arg note "$F_DET_NOTE" \
    --arg rbn "$F_ROLLBACK_NOTE" --arg hn "$F_HTTP_NOTE" \
    --argjson d "$F_DETECT" --argjson b "$F_BUILD" --argjson r "$F_RUN" --argjson h "$F_HTTP" \
    --argjson hl "$F_HEALTH" --argjson rd "$F_REDEPLOY" --argjson rb "$F_ROLLBACK" --argjson s "$secs" \
    --argjson qw "$qwait" --argjson bs "$F_BUILD_SECS" --argjson cc "$F_CONCURRENT" \
    '{name:$name,app:$app,detect_ok:$d,build_ok:$b,run_ok:$r,http_ok:$h,health_ok:$hl,
      redeploy_ok:$rd,rollback_ok:$rb,seconds:$s,queue_wait_seconds:$qw,build_seconds:$bs,
      concurrent_builds_at_start:$cc,rollback_note:$rbn,http_note:$hn,
      failure_stage:(if $stage=="" then null else $stage end),
      failure_excerpt:(if $ex=="" then null else $ex end),
      detected:{framework:$fw,port:$port,build:$bt,note:$note}}' >"$RUN_DIR/$WORKER.json.tmp"
  mv -f "$RUN_DIR/$WORKER.json.tmp" "$RUN_DIR/$WORKER.json"
  set_state "$WORKER" "$final"
  exit 0
fi

# ---------------------------------------------------------------- orchestrator
FX_LIST=$(jq -r ".${FX_KEY}[].name" "$FIXTURES")
if [ -n "$WAVE" ]; then
  FX_LIST=$(jq -r --argjson w "$WAVE" ".${FX_KEY}[] | select(.wave==\$w) | .name" "$FIXTURES")
fi
if [ -n "$ONLY" ]; then
  want=",${ONLY},"
  FX_LIST=$(for n in $FX_LIST; do case "$want" in *",$n,"*) echo "$n" ;; esac; done)
fi
if [ -n "$FAILED_FROM" ]; then
  failed=$(for f in "$OUT_DIR/$FAILED_FROM"/*.json; do
    [ -f "$f" ] && jq -r 'select(.failure_stage != null and .failure_stage != "unsupported") | .name' "$f"
  done)
  FX_LIST=$(for n in $FX_LIST; do for m in $failed; do [ "$n" = "$m" ] && echo "$n"; done; done)
fi
if [ -n "$RESUME" ]; then
  RUN_ID="$RESUME"
  FX_LIST=$(for n in $FX_LIST; do
    f="$OUT_DIR/$RESUME/$n.json"
    if [ -f "$f" ] && jq -e '.failure_stage == null' "$f" >/dev/null 2>&1; then continue; fi
    echo "$n"
  done)
fi
if [ -n "$REPORT" ]; then
  RUN_ID="$REPORT"
  FX_LIST=$(for f in "$OUT_DIR/$REPORT"/*.json; do [ -f "$f" ] && jq -r .name "$f"; done)
fi
[ -n "$FX_LIST" ] || { echo "no fixtures selected" >&2; exit 2; }

[ -n "$RUN_ID" ] || RUN_ID=$(date +%Y%m%d-%H%M%S)
RUN_DIR="$OUT_DIR/$RUN_ID"
STATE_DIR="$RUN_DIR/state"
mkdir -p "$STATE_DIR" "$RUN_DIR/building"
rm -f "$RUN_DIR/building"/*

COUNT=$(printf '%s\n' "$FX_LIST" | wc -l | tr -d ' ')
if [ "$LOAD" -eq 1 ]; then JOBS="$COUNT"; fi
[ "$JOBS" -ge 1 ] 2>/dev/null || JOBS=1

RUN_START=$(now)
if [ -z "$REPORT" ]; then
  for n in $FX_LIST; do rm -f "$RUN_DIR/$n.json"; set_state "$n" queued; done
  echo "run $RUN_ID: $COUNT fixtures, $JOBS concurrent, host $API_HOST" >&2
fi

for n in $([ -z "$REPORT" ] && echo "$FX_LIST"); do
  while [ "$(jobs -r | wc -l | tr -d ' ')" -ge "$JOBS" ]; do sleep 1; done
  wflags=()
  [ "$KEEP" -eq 1 ] && wflags+=(--keep)
  [ "$HOST_HEADER" -eq 1 ] && wflags+=(--host-header)
  [ "$I_KNOW" -eq 1 ] && wflags+=(--i-know)
  [ "$SELFTEST" -eq 1 ] && wflags+=(--selftest)
  bash "$0" --worker "$n" --run-id "$RUN_ID" --queued-at "$RUN_START" --timeout "$TIMEOUT" \
    ${wflags[@]+"${wflags[@]}"} >"$RUN_DIR/$n.log" 2>&1 &
done
wait
WALL=$(($(now) - RUN_START))
if [ -n "$REPORT" ] && [ -f "$OUT_DIR/$REPORT.json" ]; then
  WALL=$(jq -r '.load.wall_seconds // 0' "$OUT_DIR/$REPORT.json"); JOBS=$(jq -r '.load.jobs // 1' "$OUT_DIR/$REPORT.json")
fi

ROWS="$TMP/rows.jsonl"
: >"$ROWS"
for n in $FX_LIST; do
  if [ -f "$RUN_DIR/$n.json" ]; then
    cat "$RUN_DIR/$n.json" >>"$ROWS"
  else
    jq -nc --arg n "$n" '{name:$n,detect_ok:false,build_ok:false,run_ok:false,http_ok:false,health_ok:false,
      redeploy_ok:null,rollback_ok:null,seconds:0,queue_wait_seconds:0,build_seconds:0,concurrent_builds_at_start:0,
      failure_stage:"worker",failure_excerpt:"worker produced no result, see log",detected:{framework:"",port:"",build:"",note:""}}' >>"$ROWS"
  fi
done

RESULT_JSON="$OUT_DIR/$RUN_ID.json"
RESULT_MD="$OUT_DIR/$RUN_ID.md"
LR_VERSION=$(lr version 2>/dev/null | sed -n 's/^running version: *//p' | head -1)
LR_COMMIT=$(git -C "$REPO_ROOT" rev-parse --short HEAD 2>/dev/null)
RUN_DATE=$(date +%Y-%m-%d)
jq -s --arg id "$RUN_ID" --arg api "$API_HOST" --arg profile "${APP_PROFILE:-default}" \
  --arg ver "${LR_VERSION:-unknown}" --arg commit "${LR_COMMIT:-unknown}" --arg date "$RUN_DATE" \
  --argjson jobs "$JOBS" --argjson wall "$WALL" \
  '{run_id:$id,date:$date,api_host:$api,profile:$profile,levelrail_version:$ver,commit:$commit,
    load:{jobs:$jobs,wall_seconds:$wall,fixtures:length,
      peak_concurrent_builds:([.[].concurrent_builds_at_start]|max),
      max_queue_wait_seconds:([.[].queue_wait_seconds]|max),
      max_build_seconds:([.[].build_seconds]|max),
      failures:([.[]|select(.failure_stage!=null and .failure_stage!="unsupported")]|length),
      unsupported:([.[]|select(.failure_stage=="unsupported")]|length)},
    results:.}' "$ROWS" >"$RESULT_JSON"

mark() { case "$1" in true) echo "pass" ;; unsupported) echo "unsupported" ;; false) echo "FAIL" ;; *) echo "-" ;; esac; }
TAB=$(printf '\t')
{
  echo "# Deploy conformance $RUN_DATE"
  echo
  echo "Levelrail ${LR_VERSION:-unknown}, commit ${LR_COMMIT:-unknown}, host $API_HOST, run $RUN_ID."
  echo
  echo "| fixture | detected | build | runs | http | health | redeploy | rollback | seconds |"
  echo "|---|---|---|---|---|---|---|---|---|"
  jq -r '.results[] | [.name, (.detected.framework + " :" + .detected.port), (if .failure_stage=="unsupported" then "unsupported" else (.build_ok|tostring) end), (if .failure_stage=="unsupported" then "unsupported" else (.run_ok|tostring) end), (if .failure_stage=="unsupported" then "unsupported" else (.http_ok|tostring) end), (if .failure_stage=="unsupported" then "unsupported" else (.health_ok|tostring) end), (.redeploy_ok|tostring), (.rollback_ok|tostring), .seconds] | @tsv' "$RESULT_JSON" |
  while IFS="$TAB" read -r n det b r h hl rd rb sec; do
    echo "| $n | $det | $(mark "$b") | $(mark "$r") | $(mark "$h") | $(mark "$hl") | $(mark "$rd") | $(mark "$rb") | $sec |"
  done
  echo
  echo "## Load"
  echo
  jq -r '.load | "- concurrent jobs: \(.jobs)\n- wall time: \(.wall_seconds)s for \(.fixtures) fixtures\n- peak builds in flight: \(.peak_concurrent_builds)\n- max queue wait: \(.max_queue_wait_seconds)s\n- slowest build: \(.max_build_seconds)s\n- failures: \(.failures)\n- unsupported by the platform: \(.unsupported)"' "$RESULT_JSON"
  echo
  echo "| fixture | queue wait | build secs | builds in flight at start |"
  echo "|---|---|---|---|"
  jq -r '.results[] | "| \(.name) | \(.queue_wait_seconds)s | \(.build_seconds)s | \(.concurrent_builds_at_start) |"' "$RESULT_JSON"
  echo
  jq -r '.results[] | select(.failure_stage != null) | "### \(.name) (\(.failure_stage))\n\nDetected: framework=\(.detected.framework) port=\(.detected.port) build=\(.detected.build) \(.detected.note)\n\n```\n\(.failure_excerpt // "")\n```\n"' "$RESULT_JSON"
} >"$RESULT_MD"

printf '\n%-28s %-6s %-6s %-6s %-6s %-6s %-8s %-8s %5s  %s\n' fixture detect build run http health redeploy rollback secs failure
jq -r '.results[] | [.name, .detect_ok, (if .failure_stage=="unsupported" then "unsupported" else (.build_ok|tostring) end), (if .failure_stage=="unsupported" then "unsupported" else (.run_ok|tostring) end), (if .failure_stage=="unsupported" then "unsupported" else (.http_ok|tostring) end), (if .failure_stage=="unsupported" then "unsupported" else (.health_ok|tostring) end), (.redeploy_ok|tostring), (.rollback_ok|tostring), .seconds, (.failure_stage // "-")] | @tsv' "$RESULT_JSON" |
while IFS="$TAB" read -r n d b r h hl rd rb sec st; do
  printf '%-28s %-6s %-6s %-6s %-6s %-6s %-8s %-8s %5s  %s\n' "$n" "$(mark "$d")" "$(mark "$b")" "$(mark "$r")" "$(mark "$h")" "$(mark "$hl")" "$(mark "$rd")" "$(mark "$rb")" "$sec" "$st"
done
echo
echo "load: $JOBS jobs, wall ${WALL}s, peak builds in flight $(jq .load.peak_concurrent_builds "$RESULT_JSON")"
echo "results: $RESULT_JSON"
echo "matrix:  $RESULT_MD"
echo "logs:    $RUN_DIR/<name>.log"

bad=$(jq '.load.failures' "$RESULT_JSON")
[ "$bad" -eq 0 ] && exit 0
exit 1
