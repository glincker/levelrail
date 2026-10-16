#!/usr/bin/env bash
# Edge-case suite: image-based apps named qa-edge-*, driven through the real CLI.
# Usage: APP_PROFILE=<profile> test/conformance/edge.sh [--only 1,2,5] [--recreate]
#   [--timeout 120] [--run-id ID] [--i-know]
# Apps are left running (except scenario 9's deletion target). bash 3.2 safe.
set -u

HERE=$(cd "$(dirname "$0")" && pwd)
REPO_ROOT=$(cd "$HERE/../.." && pwd)
ALLOWED="$HERE/allowed-hosts"
OUT_DIR="$HERE/results"

ONLY=""; RECREATE=0; WAIT=120; RUN_ID=""; I_KNOW=0
while [ $# -gt 0 ]; do
  case "$1" in
    --only) ONLY="${2:-}"; shift 2 ;;
    --recreate) RECREATE=1; shift ;;
    --timeout) WAIT="${2:-120}"; shift 2 ;;
    --run-id) RUN_ID="${2:-}"; shift 2 ;;
    --i-know) I_KNOW=1; shift ;;
    -h|--help) sed -n 2,5p "$0" | sed 's/^# \{0,1\}//'; exit 0 ;;
    *) echo "unknown flag: $1" >&2; exit 2 ;;
  esac
done

for tool in jq curl docker; do
  command -v "$tool" >/dev/null 2>&1 || { echo "missing tool: $tool" >&2; exit 2; }
done

TMP=$(mktemp -d "${TMPDIR:-/tmp}/lr-edge.XXXXXX")
trap 'rm -f "$TMP/loop.run"; rm -rf "$TMP"' EXIT

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
API_HOST=$(printf '%s' "$API_URL" | sed -e 's#^[a-zA-Z]*://##' -e 's#[/?].*$##' -e 's#^.*@##' -e 's#:[0-9]*$##')
host_allowed() {
  grep -v '^[[:space:]]*#' "$ALLOWED" 2>/dev/null | tr -d '[:blank:]' | grep -qxF -- "$1"
}
if ! host_allowed "$API_HOST" && [ "$I_KNOW" -ne 1 ]; then
  echo "refusing: API host '$API_HOST' is not in $ALLOWED (pass --i-know to override)" >&2
  exit 3
fi
case "$API_HOST" in localhost|127.0.0.1) ;; *) echo "edge suite only supports a local plane" >&2; exit 3 ;; esac

now() { date +%s; }
TS=$(date +%Y%m%d-%H%M%S)
[ -n "$RUN_ID" ] || RUN_ID="edge-$TS"
ROWS="$TMP/rows.jsonl"; : >"$ROWS"
APPS_SEEN="$TMP/apps.txt"; : >"$APPS_SEEN"
EVF=""
OUT=""
RC=0

# ---------------------------------------------------------------- evidence
ev_text() { [ -n "$EVF" ] && printf '%s\n' "$*" >>"$EVF"; return 0; }
# evc: run the CLI, log the command and a trimmed output, set OUT and RC.
evc() {
  OUT=$("$CLI" "$@" 2>&1); RC=$?
  ev_text "\$ levelrail-cli $* (exit $RC)"
  [ -n "$OUT" ] && ev_text "$(printf '%s' "$OUT" | head -c 700)"
  return 0
}
# quiet: run the CLI without logging evidence.
quiet() { "$CLI" "$@" 2>&1; }
begin() { EVF="$TMP/ev.$1"; : >"$EVF"; echo "==> scenario $1: $2" >&2; }
verdict() { # id name verdict summary [bug_hint]
  local ev
  ev=$(head -c 6000 "$EVF" 2>/dev/null)
  jq -nc --arg id "$1" --arg name "$2" --arg v "$3" --arg s "$4" --arg b "${5:-}" --arg e "$ev" \
    '{id:($id|tonumber),scenario:$name,verdict:$v,summary:$s,suspected_bug:$b,evidence:$e}' >>"$ROWS"
  echo "    $3: $4" >&2
}
selected() { [ -z "$ONLY" ] && return 0; case ",$ONLY," in *",$1,"*) return 0 ;; esac; return 1; }

# ---------------------------------------------------------------- helpers
app_exists() { "$CLI" apps get "$1" --json >/dev/null 2>&1; }
note_app() { printf '%s\n' "$1" >>"$APPS_SEEN"; }

# resolve the real service name: compose imports are named <app>-<service>.
resolve_app() { # base
  if app_exists "$1"; then echo "$1"; return; fi
  "$CLI" apps list --json 2>/dev/null | jq -r --arg p "$1-" '.[].name | select(startswith($p))' | head -1
}

# fresh: make sure no app named like $1 exists; honours --recreate.
fresh() { # base
  local n
  n=$(resolve_app "$1")
  [ -z "$n" ] && return 0
  if [ "$RECREATE" -eq 1 ]; then
    case "$n" in qa-edge-*) "$CLI" apps delete "$n" --delete-data --confirm "$n" >/dev/null 2>&1; sleep 4; return 0 ;; esac
  fi
  ev_text "app $n already exists (pass --recreate to replace it)"
  return 1
}

wait_running() { # app seconds
  local end r
  end=$(($(now) + ${2:-$WAIT}))
  while [ "$(now)" -lt "$end" ]; do
    r=$("$CLI" apps network "$1" --json 2>/dev/null | jq -r '.running // false')
    [ "$r" = "true" ] && return 0
    sleep 2
  done
  return 1
}

host_port() { "$CLI" apps network "$1" --json 2>/dev/null | jq -r '.host_port // empty'; }
ready_reason() { "$CLI" apps status "$1" --json 2>/dev/null | jq -r '.[] | select(.Type=="Ready") | "\(.Status)/\(.Reason): \(.Message)"'; }
cid() { docker ps -q --filter "name=^$1" | head -1; }

# http_get: GET the app's host port, sets CODE and BODY.
CODE=""; BODY=""
http_get() { # app path
  local p
  p=$(host_port "$1")
  if [ -z "$p" ]; then CODE="000"; BODY=""; return 1; fi
  BODY=$(curl -s -m 8 -o - -w '\n%{http_code}' "http://127.0.0.1:${p}${2:-/}" 2>/dev/null)
  CODE=$(printf '%s' "$BODY" | tail -n1)
  BODY=$(printf '%s' "$BODY" | sed '$d')
  [ "$CODE" = "200" ]
}

http_wait() { # app path seconds
  local end
  end=$(($(now) + ${3:-60}))
  while [ "$(now)" -lt "$end" ]; do
    http_get "$1" "${2:-/}" && return 0
    sleep 2
  done
  return 1
}

deploy_image() { # app image port [extra import flags...]
  local app="$1" img="$2" port="$3"; shift 3
  evc import "$img" --name "$app" --port "$port" --deploy --json "$@"
  [ "$RC" -eq 0 ] && note_app "$app"
  return "$RC"
}

free_port() { # start
  local p="$1"
  while [ "$p" -lt 65000 ]; do
    if ! (exec 3<>"/dev/tcp/127.0.0.1/$p") 2>/dev/null; then echo "$p"; return; fi
    p=$((p + 1))
  done
}

newest_deploy() { "$CLI" apps deploys list "$1" --json 2>/dev/null | jq -r '.[0].id // empty'; }
wait_deploy_terminal() { # app seconds ; echoes status
  local end id st
  end=$(($(now) + ${2:-$WAIT}))
  while [ "$(now)" -lt "$end" ]; do
    id=$(newest_deploy "$1")
    st=$("$CLI" apps deploys show "$1" "$id" --json 2>/dev/null | jq -r '.status // empty')
    case "$st" in succeeded|failed|canceled|cancelled|superseded|blocked) echo "$st"; return 0 ;; esac
    sleep 2
  done
  echo timeout
  return 1
}

start_loop() {
  : >"$TMP/loop.out"; : >"$TMP/loop.run"
  (
    while [ -f "$TMP/loop.run" ]; do
      np=$("$CLI" apps network "$app" --json 2>/dev/null | jq -r '.host_port // empty')
      [ -n "$np" ] && lp="$np"
      if [ -n "${lp:-}" ]; then curl -s -m 3 -o /dev/null -w '%{http_code}\n' "http://127.0.0.1:${lp}/" >>"$TMP/loop.out" 2>/dev/null; else echo 000 >>"$TMP/loop.out"; fi
      sleep 0.3
    done
  ) &
  LOOP_PID=$!
}
stop_loop() { rm -f "$TMP/loop.run"; wait "$LOOP_PID" 2>/dev/null; }

# ---------------------------------------------------------------- scenarios
s1_crashloop() {
  local app="qa-edge-crashloop" svc restarts reason logs i
  begin 1 crashloop
  fresh "$app" || { verdict 1 crashloop SKIP "app exists, use --recreate"; return; }
  evc import --docker-run "docker run -p 8080 busybox:1.36 sh -c 'echo edge-boom >&2; exit 1'" \
    --name "$app" --deploy --json
  if [ "$RC" -ne 0 ]; then verdict 1 crashloop FAIL "import --deploy failed: $(printf '%s' "$OUT" | head -c 200)"; return; fi
  sleep 3
  svc=$(resolve_app "$app")
  [ -n "$svc" ] || svc="$app"
  note_app "$svc"
  ev_text "service name: $svc"
  sleep 57
  reason=$(ready_reason "$svc")
  ev_text "Ready condition after 60s: $reason"
  restarts=$(printf '%s' "$reason" | sed -n 's/.*(\([0-9][0-9]*\) restarts).*/\1/p')
  i=$(docker ps -a --filter "name=^$svc" --format '{{.Names}}' | wc -l | tr -d ' ')
  ev_text "docker containers matching $svc: $i; restart count in message: ${restarts:-none}"
  evc apps logs "$svc" --tail 5
  logs="$OUT"
  evc apps timeline "$svc" --limit 5
  case "$reason" in
    *CrashLoop*)
      if [ -n "$restarts" ] && [ "$restarts" -gt 15 ]; then
        verdict 1 crashloop FAIL "crashloop condition present but $restarts restarts in 60s looks like a hot loop"
      elif printf '%s' "$logs" | grep -q edge-boom; then
        verdict 1 crashloop PASS "CrashLoopBackOff condition, ${restarts:-?} restarts in 60s, backoff message, logs show edge-boom"
      else
        verdict 1 crashloop WARN "CrashLoopBackOff with ${restarts:-?} restarts in 60s, but the container's stderr is not in apps logs" \
          "log collector for crashed containers: nothing stored after the container exits"
      fi ;;
    *) verdict 1 crashloop FAIL "no CrashLoop condition: $reason" ;;
  esac
}

s2_wrong_port() {
  local app="qa-edge-wrongport" reason
  begin 2 wrong-port
  fresh "$app" || { verdict 2 wrong-port SKIP "app exists, use --recreate"; return; }
  deploy_image "$app" traefik/whoami 3000 || true
  sleep 50
  reason=$(ready_reason "$app")
  ev_text "Ready after deploy with wrong port: $reason"
  evc apps network "$app" --json
  evc apps timeline "$app" --limit 4
  reason="$reason $(quiet apps timeline "$app" --limit 2)"
  http_get "$app" / ; ev_text "GET / on host port: code=$CODE"
  evc apps exposure "$app" --port 80
  if printf '%s' "$OUT" | grep -qi "404 page not found"; then
    if [ "$CODE" != "200" ] && ! printf '%s' "$reason" | grep -qi -e "listening" -e "port" -e "refused" -e "unreachable"; then
      verdict 2 wrong-port FAIL "silent failure: deploy succeeded, network running=true, GET / code=$CODE, no port or readiness message anywhere (Ready: '${reason:-empty}'); exposure --port fix BLOCKED, plane has no exposure endpoint" \
        "no readiness check when no probe path is set: add a TCP connect check against the declared port and a PortNotListening condition"
    else
      verdict 2 wrong-port BLOCKED "port problem is reported ($reason) but the exposure fix is blocked: plane has no exposure endpoint" \
        "scratch plane binary lacks /api/v1/apps/{name}/exposure"
    fi
    return
  fi
  if [ "$RC" -ne 0 ]; then verdict 2 wrong-port FAIL "apps exposure --port 80 failed: $(printf '%s' "$OUT" | head -c 200)"; return; fi
  if http_wait "$app" / 45; then
    ev_text "after exposure fix: GET / code=$CODE"
    verdict 2 wrong-port PASS "port change applied live, app answers 200 (first-half state: $reason)"
  else
    ev_text "after exposure fix: still code=$CODE; docker: $(docker ps -a --filter "name=^$app" --format '{{.Names}} {{.Ports}}' | tr '\n' ';')"
    ev_text "container_port now: $(quiet apps network "$app" --json | jq -r '.container_port')"
    evc apps restart "$app"
    if http_wait "$app" / 60; then
      verdict 2 wrong-port FAIL "exposure --port 80 saved the change (container_port 3000 -> 80) but did not recreate the container: still published on 3000, app dead until an explicit restart (first-half state: $reason)" \
        "exposure set: the port change does not mark the app dirty or trigger a recreate; reconciler sees AlreadyRunning"
    else
      verdict 2 wrong-port FAIL "port changed but app still not answering after an explicit restart (code=$CODE)"
    fi
  fi
}

s3_wrong_health() {
  local app="qa-edge-health" st id fail total bad b_total b_bad
  begin 3 wrong-health-path
  fresh "$app" || { verdict 3 wrong-health-path SKIP "app exists, use --recreate"; return; }
  deploy_image "$app" nginx:1.25-alpine 80 || true
  if ! http_wait "$app" / 60; then verdict 3 wrong-health-path FAIL "baseline app never answered 200"; return; fi
  ev_text "GET /nope on this image: $(curl -s -m 5 -o /dev/null -w '%{http_code}' "http://127.0.0.1:$(host_port "$app")/nope")"
  # baseline: a healthy redeploy, to know what zero downtime looks like on this plane
  start_loop
  evc apps deploy "$app" --image nginx:1.26-alpine
  wait_deploy_terminal "$app" 120 >/dev/null
  sleep 25
  stop_loop
  b_total=$(wc -l <"$TMP/loop.out" | tr -d ' '); b_bad=$(grep -vc '^200$' "$TMP/loop.out")
  ev_text "baseline healthy redeploy: $b_total requests, $b_bad non-200 ($(sort "$TMP/loop.out" | uniq -c | tr '\n' ' '))"
  evc apps health set "$app" --probe readiness --path /nope --ready-timeout 30s
  start_loop
  evc apps deploy "$app" --image nginx:1.27-alpine
  st=$(wait_deploy_terminal "$app" 150)
  sleep 60
  stop_loop
  id=$(newest_deploy "$app")
  ev_text "newest deploy $id status at first terminal poll: $st"
  st=$(quiet apps deploys show "$app" "$id" --json | jq -r '.status // empty')
  ev_text "newest deploy $id status after settling: $st"
  evc apps deploys show "$app" "$id" --json
  fail=$(printf '%s' "$OUT" | jq -r '.failure | if . then "\(.failing_step // ""): \(.cause // "")" else "" end' 2>/dev/null)
  total=$(wc -l <"$TMP/loop.out" | tr -d ' ')
  bad=$(grep -vc '^200$' "$TMP/loop.out")
  ev_text "curl loop during bad redeploy: $total requests, $bad non-200 ($(sort "$TMP/loop.out" | uniq -c | tr '\n' ' '))"
  ev_text "containers still up: $(docker ps --filter "name=^$app" --format '{{.Image}}' | tr '\n' ' ')"
  ev_text "running image: $(quiet apps get "$app" --json | jq -r '.image')  docker: $(docker ps -a --filter "name=^$app" --format '{{.Image}} {{.Status}}' | tr '\n' ';')"
  evc apps status "$app" --json
  ev_text "Ready: $(ready_reason "$app")"
  evc apps timeline "$app" --limit 5
  if http_get "$app" /; then
    case "$st" in
      failed|blocked|canceled)
        if [ "$bad" -eq 0 ] && [ -n "$fail" ]; then
          verdict 3 wrong-health-path PASS "deploy $st with reason '$fail', old version kept serving, 0 of $total requests failed"
        elif [ "$bad" -eq 0 ]; then
          verdict 3 wrong-health-path WARN "deploy $st and zero downtime, but no readable failure reason on the deploy" \
            "deploys show .failure empty for a readiness timeout"
        else
          verdict 3 wrong-health-path FAIL "deploy $st but $bad of $total requests failed during the redeploy (baseline healthy redeploy: $b_bad of $b_total)" \
            "cutover or old container teardown before readiness passed"
        fi ;;
      succeeded) verdict 3 wrong-health-path FAIL "deploy reported succeeded although readiness path /nope returns 404; $bad of $total requests failed (baseline: $b_bad of $b_total)" \
        "readiness probe not applied to the redeploy, or a 404 is accepted" ;;
      *) verdict 3 wrong-health-path FAIL "deploy ended '$st', not a clear failure" ;;
    esac
  else
    verdict 3 wrong-health-path FAIL "app stopped serving after the bad redeploy (code=$CODE), $bad of $total loop requests failed (baseline healthy redeploy: $b_bad of $b_total)" \
      "failed redeploy took the old version down"
  fi
}

s4_env() {
  local app="qa-edge-env" c1 c2 v secret f leaks o
  begin 4 env-change
  fresh "$app" || { verdict 4 env-change SKIP "app exists, use --recreate"; return; }
  deploy_image "$app" nginx:1.27-alpine 80 --env GREETING=one || true
  wait_running "$app" 60 || { verdict 4 env-change FAIL "app never ran"; return; }
  c1=$(cid "$app")
  evc apps exec "$app" -- sh -c 'printenv GREETING'
  f="$TMP/env4.env"; printf 'GREETING=two\nEXTRA=added\n' >"$f"
  evc apps env import "$app" --file "$f" --apply
  sleep 8
  wait_running "$app" 60 || true
  c2=$(cid "$app")
  evc apps exec "$app" -- sh -c 'printenv GREETING; printenv EXTRA'
  v="$OUT"
  ev_text "container before=$c1 after=$c2"
  secret="s3cr3t-edge-$$-value"
  evc apps secrets set "$app" TOPSECRET "$secret" --apply
  sleep 8
  wait_running "$app" 60 || true
  evc apps exec "$app" -- sh -c 'printenv TOPSECRET | wc -c'
  leaks=""
  for o in "get $app --json" "status $app --json" "timeline $app --limit 30" "logs $app --tail 50" "env export $app" "secrets list $app" "deploys list $app --json"; do
    # shellcheck disable=SC2086
    if quiet apps $o | grep -qF "$secret"; then leaks="$leaks [apps $o]"; fi
  done
  ev_text "secret value found in:${leaks:- nowhere}"
  if printf '%s' "$v" | grep -q two && printf '%s' "$v" | grep -q added && [ "$c1" != "$c2" ]; then
    if [ -z "$leaks" ]; then
      verdict 4 env-change PASS "env change rolled the container ($c1 -> $c2), new values visible, secret never echoed"
    else
      verdict 4 env-change FAIL "secret value echoed by:$leaks" "secret masking on the listed read paths"
    fi
  else
    verdict 4 env-change FAIL "env change not visible or container not replaced: $(printf '%s' "$v" | tr '\n' ' ')" \
      "env import --apply restart"
  fi
}

s5_sameport() {
  local i app ports ok
  begin 5 same-container-port-x4
  ports=""; ok=1
  i=1
  while [ $i -le 4 ]; do
    app="qa-edge-sameport-$i"
    if fresh "$app"; then
      ( "$CLI" import traefik/whoami --name "$app" --port 8080 --env WHOAMI_PORT_NUMBER=8080 --deploy --json >"$TMP/s5.$i" 2>&1 ) &
      note_app "$app"
    fi
    i=$((i + 1))
  done
  wait
  i=1
  while [ $i -le 4 ]; do
    app="qa-edge-sameport-$i"
    if wait_running "$app" 90 && http_wait "$app" / 30; then
      ev_text "$app host_port=$(host_port "$app") GET / -> $CODE ($(printf '%s' "$BODY" | head -1))"
      ports="$ports $(host_port "$app")"
    else
      ok=0; ev_text "$app not serving: $(ready_reason "$app")  create: $(head -c 200 "$TMP/s5.$i" 2>/dev/null)"
    fi
    i=$((i + 1))
  done
  ev_text "host ports:$ports"
  if [ "$ok" -eq 1 ] && [ "$(printf '%s' "$ports" | tr ' ' '\n' | grep -v '^$' | sort -u | wc -l | tr -d ' ')" -eq 4 ]; then
    verdict 5 same-container-port-x4 PASS "4 apps all on container port 8080, distinct host ports:$ports"
  else
    verdict 5 same-container-port-x4 FAIL "not all four apps ran on distinct ports (ports:$ports)"
  fi
}

s6_hostport_conflict() {
  local p a b
  begin 6 fixed-host-port-conflict
  a="qa-edge-pin-a"; b="qa-edge-pin-b"
  fresh "$a" || { verdict 6 fixed-host-port-conflict SKIP "app exists, use --recreate"; return; }
  fresh "$b" || { verdict 6 fixed-host-port-conflict SKIP "app exists, use --recreate"; return; }
  p=$(free_port 18700)
  ev_text "pinned host port: $p"
  evc apps create --name "$a" --image traefik/whoami --port 80 --host-port "$p" --json
  [ "$RC" -eq 0 ] && note_app "$a"
  wait_running "$a" 60 || true
  ev_text "$a serving on $(host_port "$a"): $(curl -s -m 5 "http://127.0.0.1:${p}/" | head -1)"
  evc apps create --name "$b" --image traefik/whoami --port 80 --host-port "$p" --json
  if [ "$RC" -ne 0 ]; then
    if printf '%s' "$OUT" | grep -q -e "$a" -e "$p"; then
      verdict 6 fixed-host-port-conflict PASS "second app refused and the message names the holder or port: $(printf '%s' "$OUT" | jq -r '.error // .' 2>/dev/null | head -c 220)"
    else
      verdict 6 fixed-host-port-conflict WARN "second app refused but the error does not say who holds $p: $(printf '%s' "$OUT" | head -c 200)" \
        "port-conflict error text lacks holder app name"
    fi
  else
    note_app "$b"
    sleep 10
    ev_text "$b state: $(ready_reason "$b")"
    ev_text "apps get $b: $(quiet apps get "$b" --json | jq -c '{name,port,host_port}' 2>/dev/null)"
    verdict 6 fixed-host-port-conflict FAIL "second app with the same pinned host port $p was accepted (state: $(ready_reason "$b"))" \
      "create does not validate --host-port against ports held by other apps"
  fi
}

s7_direct() {
  local app="qa-edge-direct" hp lan rules
  begin 7 direct-ip-port
  fresh "$app" || { verdict 7 direct-ip-port SKIP "app exists, use --recreate"; return; }
  deploy_image "$app" traefik/whoami 80 || true
  wait_running "$app" 60 || true
  evc apps exposure "$app"
  if printf '%s' "$OUT" | grep -qi "404 page not found"; then
    verdict 7 direct-ip-port BLOCKED "control plane has no exposure endpoint (plane binary older than the feature); nothing to verify" \
      "scratch plane at 18080 runs a binary without /api/v1/apps/{name}/exposure; rebuild plane to retest"
    return
  fi
  evc firewall list
  evc apps exposure "$app" --mode direct --json
  [ "$RC" -eq 0 ] || { verdict 7 direct-ip-port FAIL "set direct failed: $(printf '%s' "$OUT" | head -c 200)"; return; }
  sleep 10; wait_running "$app" 60 || true
  evc apps exposure "$app" --json
  hp=$(printf '%s' "$OUT" | jq -r '.host_port // empty')
  [ -n "$hp" ] || hp=$(host_port "$app")
  evc firewall list
  rules="$OUT"
  ev_text "loopback: $(curl -s -m 5 -o /dev/null -w '%{http_code}' "http://127.0.0.1:${hp}/")"
  lan=$(ipconfig getifaddr en0 2>/dev/null || ipconfig getifaddr en1 2>/dev/null || true)
  [ -n "$lan" ] && ev_text "LAN $lan:$hp -> $(curl -s -m 5 -o /dev/null -w '%{http_code}' "http://${lan}:${hp}/") (record only)"
  evc apps exposure "$app" --mode proxied --json
  sleep 10; wait_running "$app" 60 || true
  evc firewall list
  if printf '%s' "$rules" | grep -q "$hp" && ! printf '%s' "$OUT" | grep -q "$hp"; then
    verdict 7 direct-ip-port PASS "direct mode recorded a rule for $hp, reachable on loopback, proxied removed it"
  else
    verdict 7 direct-ip-port WARN "see evidence: firewall rule record before/after not conclusive for port $hp" \
      "firewall list output format or rule cleanup on reversal"
  fi
}

s8_rollback() {
  local app="qa-edge-rollback" d1 s1 s2 s3
  begin 8 rollback-two-images
  fresh "$app" || { verdict 8 rollback-two-images SKIP "app exists, use --recreate"; return; }
  deploy_image "$app" nginx:1.25-alpine 80 || true
  http_wait "$app" / 60 || { verdict 8 rollback-two-images FAIL "v1 never answered"; return; }
  d1=$(newest_deploy "$app")
  s1=$(curl -sI -m 5 "http://127.0.0.1:$(host_port "$app")/" | tr -d '\r' | grep -i '^server:')
  ev_text "v1 header: $s1 (deploy $d1)"
  evc apps deploy "$app" --image nginx:1.27-alpine
  wait_deploy_terminal "$app" 120 >/dev/null
  sleep 5
  http_wait "$app" / 60 || true
  s2=$(curl -sI -m 5 "http://127.0.0.1:$(host_port "$app")/" | tr -d '\r' | grep -i '^server:')
  ev_text "v2 header: $s2"
  evc apps deploys rollback-to "$app" "$d1"
  sleep 4
  wait_deploy_terminal "$app" 120 >/dev/null
  http_wait "$app" / 60 || true
  s3=$(curl -sI -m 5 "http://127.0.0.1:$(host_port "$app")/" | tr -d '\r' | grep -i '^server:')
  ev_text "after rollback header: $s3"
  if [ "$s1" = "$s3" ] && [ "$s1" != "$s2" ]; then
    verdict 8 rollback-two-images PASS "v1 '$s1' -> v2 '$s2' -> rollback '$s3'"
  else
    verdict 8 rollback-two-images FAIL "headers v1='$s1' v2='$s2' after='$s3'" "rollback-to did not restore the first image"
  fi
}

net_for() { docker network ls --format '{{.Name}}' | grep -i -- "$1"; }
s9_lifecycle() {
  local app="qa-edge-lifecycle" del="qa-edge-deltest" c1 c2 left
  begin 9 stop-start-restart-delete
  fresh "$app" || { verdict 9 lifecycle SKIP "app exists, use --recreate"; return; }
  fresh "$del" || true
  deploy_image "$app" nginx:1.27-alpine 80 || true
  evc apps volumes attach "$app" --name data --path /data
  evc apps restart "$app"
  http_wait "$app" / 60 || { verdict 9 lifecycle FAIL "app did not come up"; return; }
  local stopped_ok=1 t0 w
  evc apps stop "$app"
  t0=$(now); w=0
  while [ "$(now)" -lt $((t0 + 60)) ]; do
    if [ "$(quiet apps network "$app" --json | jq -r '.running')" = "false" ]; then w=1; break; fi
    sleep 2
  done
  [ "$w" -eq 1 ] || stopped_ok=0
  ev_text "stop took $(($(now) - t0))s to show running=false (ok=$w), containers=$(docker ps -q --filter "name=^$app" | wc -l | tr -d ' ')"
  evc apps start "$app"
  http_wait "$app" / 60 || stopped_ok=0
  c1=$(cid "$app")
  evc apps restart "$app"; sleep 6
  http_wait "$app" / 60 || stopped_ok=0
  c2=$(cid "$app")
  ev_text "container after start=$c1 after restart=$c2"
  deploy_image "$del" nginx:1.27-alpine 80 || true
  evc apps volumes attach "$del" --name keep --path /keep
  evc apps restart "$del"; sleep 8
  ev_text "before delete: containers=$(docker ps -a --filter "name=$del" --format '{{.Names}}' | tr '\n' ' ') volumes=$(docker volume ls --format '{{.Name}}' | grep -- "$del" | tr '\n' ' ') networks=$(net_for "$del" | tr '\n' ' ')"
  evc apps delete "$del" --delete-data --confirm "$del" --json
  sleep 15
  left="containers=$(docker ps -a --filter "name=$del" --format '{{.Names}}' | tr '\n' ' ') volumes=$(docker volume ls --format '{{.Name}}' | grep -- "$del" | tr '\n' ' ') networks=$(net_for "$del" | tr '\n' ' ')"
  ev_text "after delete+15s: $left"
  if [ "$stopped_ok" -eq 1 ] && [ "$c1" != "$c2" ] && printf '%s' "$left" | grep -q 'containers= volumes= networks=$'; then
    verdict 9 lifecycle PASS "stop/start/restart behaved, delete removed container, volume and network"
  elif [ "$stopped_ok" -eq 1 ]; then
    verdict 9 lifecycle FAIL "stop/start/restart fine but delete left: $left" \
      "teardown after delete: remove containers/volume/network (see apps delete teardown_pending output)"
  else
    verdict 9 lifecycle FAIL "stop/start/restart misbehaved, see evidence; delete leftovers: $left"
  fi
}

s10_burst() {
  local i n app okc up t0 t1 nets0 nets1 pool created
  begin 10 burst-12
  nets0=$(docker network ls -q | wc -l | tr -d ' ')
  t0=$(now)
  i=1
  while [ $i -le 12 ]; do
    n=$(printf '%02d' $i); app="qa-edge-burst-$n"
    if fresh "$app"; then
      ( "$CLI" import traefik/whoami --name "$app" --port 80 --deploy --json >"$TMP/s10.$n" 2>&1; echo "rc=$?" >>"$TMP/s10.$n" ) &
      note_app "$app"
    fi
    i=$((i + 1))
  done
  wait
  created=$(($(now) - t0))
  up=0; okc=0; pool=0
  i=1
  while [ $i -le 12 ]; do
    n=$(printf '%02d' $i); app="qa-edge-burst-$n"
    grep -q 'rc=0' "$TMP/s10.$n" 2>/dev/null && okc=$((okc + 1))
    if grep -qi -e "pool" -e "CreateFailed" -e "address pools" "$TMP/s10.$n" 2>/dev/null; then pool=$((pool + 1)); ev_text "$app create output: $(grep -i -e pool -e CreateFailed "$TMP/s10.$n" | head -c 300)"; fi
    i=$((i + 1))
  done
  i=1
  while [ $i -le 12 ]; do
    n=$(printf '%02d' $i); app="qa-edge-burst-$n"
    if wait_running "$app" 90; then up=$((up + 1)); else
      ev_text "$app not running: $(ready_reason "$app")"
      if ready_reason "$app" | grep -qi -e pool -e CreateFailed; then pool=$((pool + 1)); fi
    fi
    i=$((i + 1))
  done
  t1=$(($(now) - t0))
  nets1=$(docker network ls -q | wc -l | tr -d ' ')
  ev_text "create calls ok=$okc/12 (parallel create phase ${created}s); running=$up/12 after ${t1}s; networks $nets0 -> $nets1; pool-or-CreateFailed hits=$pool"
  if [ "$up" -eq 12 ] && [ "$pool" -eq 0 ]; then
    verdict 10 burst-12 PASS "12/12 running in ${t1}s, no network pool errors, networks $nets0 -> $nets1"
  else
    verdict 10 burst-12 FAIL "$up/12 running after ${t1}s, pool or CreateFailed hits=$pool, networks $nets0 -> $nets1" \
      "docker network address pool exhaustion (one bridge per app); fix is in another worktree"
  fi
}

s11_bigenv() {
  local app="qa-edge-big" f i n c1 c2 cnt data
  begin 11 big-env-many-volumes
  fresh "$app" || { verdict 11 big-env SKIP "app exists, use --recreate"; return; }
  deploy_image "$app" nginx:1.27-alpine 80 || true
  wait_running "$app" 60 || { verdict 11 big-env FAIL "base app never ran"; return; }
  f="$TMP/big.env"; : >"$f"
  i=1; while [ $i -le 50 ]; do printf 'EDGE_VAR_%02d=value-%02d-%s\n' "$i" "$i" "$(printf 'x%.0s' 1 2 3 4 5 6 7 8)" >>"$f"; i=$((i + 1)); done
  evc apps env import "$app" --file "$f"
  evc apps volumes attach "$app" --name vol-a --path /data/a
  evc apps volumes attach "$app" --name vol-b --path /data/b
  evc apps volumes attach "$app" --name vol-c --path /data/c
  evc apps restart "$app"
  sleep 8; wait_running "$app" 60 || true
  evc apps exec "$app" -- sh -c "echo persisted-$$ > /data/a/f.txt; echo bee > /data/b/f.txt; echo sea > /data/c/f.txt; cat /data/a/f.txt"
  c1=$(cid "$app")
  evc apps restart "$app"
  sleep 8; wait_running "$app" 60 || true
  c2=$(cid "$app")
  evc apps exec "$app" -- sh -c 'cat /data/a/f.txt /data/b/f.txt /data/c/f.txt; printenv | grep -c "^EDGE_VAR_"'
  data="$OUT"; cnt=$(printf '%s' "$data" | tail -n1)
  n=$(quiet apps volumes get "$app" --json | jq 'length')
  ev_text "container $c1 -> $c2; volumes=$n; env count in container=$cnt"
  if printf '%s' "$data" | grep -q "persisted-$$" && [ "$cnt" = "50" ] && [ "$n" = "3" ] && [ "$c1" != "$c2" ]; then
    verdict 11 big-env PASS "50 env vars and 3 volumes survived restart, data intact"
  else
    verdict 11 big-env FAIL "data or env lost: $(printf '%s' "$data" | tr '\n' ' ')" "env or volume remount on restart"
  fi
}

s12_sleep() {
  local app="qa-edge-sleep" st t0 t1 hp ttfb
  begin 12 sleep-when-idle
  fresh "$app" || { verdict 12 sleep SKIP "app exists, use --recreate"; return; }
  deploy_image "$app" nginx:1.27-alpine 80 || true
  http_wait "$app" / 60 || { verdict 12 sleep FAIL "app never answered"; return; }
  evc apps sleep enable "$app" --after 5m --hold
  if [ "$RC" -ne 0 ]; then verdict 12 sleep SKIP "sleep not enabled or refused on this plane: $(printf '%s' "$OUT" | head -c 200)"; return; fi
  evc apps sleep now "$app"
  t0=$(now)
  while [ "$(now)" -lt $((t0 + 40)) ] && [ "$(docker ps -q --filter "name=^$app" | wc -l | tr -d ' ')" != "0" ]; do sleep 2; done
  ev_text "container stopped $(($(now) - t0))s after sleep now"
  evc apps sleep status "$app"
  st="$OUT"
  hp=$(host_port "$app")
  ev_text "while asleep: containers=$(docker ps -q --filter "name=^$app" | wc -l | tr -d ' ') host_port=${hp:-none} running=$(quiet apps network "$app" --json | jq -r '.running')"
  ttfb=$(curl -s -m 10 -o /dev/null -w '%{http_code} in %{time_total}s' "http://127.0.0.1:${hp:-1}/" 2>/dev/null)
  ev_text "request to host port while asleep: $ttfb"
  ev_text "ingress on 127.0.0.1:80 (needed for wake-on-request): $(curl -s -m 3 -o /dev/null -w '%{http_code}' http://127.0.0.1:80/ 2>/dev/null)"
  evc apps sleep wake "$app"
  t1=$(now)
  wait_running "$app" 60 || true
  http_get "$app" / ; ev_text "after wake: code=$CODE, wake took $(($(now) - t1))s"
  if printf '%s' "$st" | grep -qi asleep && [ "$CODE" = "200" ]; then
    verdict 12 sleep WARN "sleeps (container gone, no host port) and 'sleep wake' brings it back in $(($(now) - t1))s; a direct request while asleep got $ttfb, so wake-on-first-request could not be measured without an ingress on the scratch plane" \
      "wake-on-request only exists behind the ingress; host-port access to a sleeping app is just refused"
  else
    verdict 12 sleep FAIL "sleep status or wake failed: $(printf '%s' "$st" | head -c 160), code=$CODE"
  fi
}

# ---------------------------------------------------------------- run
START=$(now)
for n in 1 2 3 4 5 6 7 8 9 10 11 12; do
  selected "$n" || continue
  case "$n" in
    1) s1_crashloop ;; 2) s2_wrong_port ;; 3) s3_wrong_health ;; 4) s4_env ;;
    5) s5_sameport ;; 6) s6_hostport_conflict ;; 7) s7_direct ;; 8) s8_rollback ;;
    9) s9_lifecycle ;; 10) s10_burst ;; 11) s11_bigenv ;; 12) s12_sleep ;;
  esac
done
ELAPSED=$(($(now) - START))

# ---------------------------------------------------------------- report
URLS="$TMP/urls.jsonl"; : >"$URLS"
sort -u "$APPS_SEEN" | while read -r a; do
  r=$(resolve_app "$a")
  [ -n "$r" ] || continue
  net=$("$CLI" apps network "$r" --json 2>/dev/null)
  hp=$(printf '%s' "$net" | jq -r '.host_port // empty')
  run=$(printf '%s' "$net" | jq -r '.running // false')
  jq -nc --arg a "$r" --arg u "${hp:+http://127.0.0.1:$hp}" --arg r "$run" --arg f "$(printf '%s' "$net" | jq -r '.fallback_url // ""')" \
    '{app:$a,url:$u,running:$r,fallback_url:$f}' >>"$URLS"
done
mkdir -p "$OUT_DIR"
JSON="$OUT_DIR/$RUN_ID.json"; MD="$OUT_DIR/$RUN_ID.md"
jq -n --arg run "$RUN_ID" --arg host "$API_HOST" --argjson secs "$ELAPSED" \
  --slurpfile rows "$ROWS" --slurpfile urls "$URLS" \
  '{run:$run,host:$host,seconds:$secs,scenarios:$rows,apps:$urls}' >"$JSON"
{
  echo "# Edge-case suite $RUN_ID"
  echo
  echo "Plane: $API_URL, wall time ${ELAPSED}s"
  echo
  echo "| # | Scenario | Verdict | Summary |"
  echo "| --- | --- | --- | --- |"
  jq -r '.scenarios | sort_by(.id)[] | "| \(.id) | \(.scenario) | \(.verdict) | \(.summary | gsub("\\|";"/") | gsub("\n";" ")) |"' "$JSON"
  echo
  jq -r '.scenarios | sort_by(.id)[] | "## \(.id). \(.scenario): \(.verdict)\n\n\(.summary)\n" + (if .suspected_bug != "" then "\nSuspected bug: \(.suspected_bug)\n" else "" end) + "\n```\n\(.evidence)\n```\n"' "$JSON"
  echo "## Live apps"
  echo
  echo "| App | URL | Running |"
  echo "| --- | --- | --- |"
  jq -r '.apps[] | "| \(.app) | \(.url) | \(.running) |"' "$JSON"
} >"$MD"

echo
jq -r '.scenarios | sort_by(.id)[] | "\(.id)\t\(.verdict)\t\(.scenario)\t\(.summary)"' "$JSON"
echo
jq -r '.apps[] | "\(.app)\t\(.url)\t\(.running)"' "$JSON"
echo "report: $MD"
jq -e '[.scenarios[] | select(.verdict=="FAIL")] | length == 0' "$JSON" >/dev/null
