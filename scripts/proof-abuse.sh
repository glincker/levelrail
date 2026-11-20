#!/usr/bin/env bash
# Real-host proof: ingress abuse defense. Replay (log_only), then enforce, on a
# throwaway app, with a small request budget and no external targets. Measures
# the per-request latency overhead of each mode against no policy. The app, and
# with it its policy, is removed on exit; no instance setting is changed.
#
# Required env:
#   LEVELRAIL_API_URL   HTTPS dashboard address of the host under test
#   LEVELRAIL_TOKEN     API token with root access (short lived)
# Optional env:
#   PROOF_APP           throwaway app name (default proof-abuse-HHMMSS)
#   PROOF_BASE_IMAGE    image to serve (default nginx:alpine)
#   PROOF_SSH           ssh target of the host: when set, the latency runs also go
#                       through the host's own address from the host (no WAN noise)
#   PROOF_N_LOCAL       requests per round from this machine (default 100)
#   PROOF_N_HOST        requests per round from PROOF_SSH (default 300)
#   PROOF_ROUNDS        interleaved rounds per mode (default 3)
set -euo pipefail

: "${LEVELRAIL_API_URL:?set LEVELRAIL_API_URL}"
: "${LEVELRAIL_TOKEN:?set LEVELRAIL_TOKEN}"
APP="${PROOF_APP:-proof-abuse-$(date +%H%M%S)}"
BASE_IMAGE="${PROOF_BASE_IMAGE:-nginx:alpine}"
SSH_TARGET="${PROOF_SSH:-}"
N_LOCAL="${PROOF_N_LOCAL:-100}"
N_HOST="${PROOF_N_HOST:-300}"
ROUNDS="${PROOF_ROUNDS:-3}"
API="${LEVELRAIL_API_URL%/}"
WORK="$(mktemp -d)"
APP_CREATED=0
FAIL=0
host=""

log() { printf '%s\n' "proof-abuse: $*"; }
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
body_of() { sed '$d'; }

cleanup() {
  set +e
  log "cleanup"
  if [ "$APP_CREATED" = 1 ]; then
    api PUT "/api/v1/apps/$APP/abuse" '{"mode":"off"}' >/dev/null
    api DELETE "/api/v1/apps/$APP" >/dev/null
  fi
  rm -rf "$WORK"
  exit "$FAIL"
}
trap cleanup EXIT

# set_policy JSON REASON: save a policy and wait until the ingress reports it applied.
set_policy() {
  local json="$1" want="$2" out reason
  out="$(api PUT "/api/v1/apps/$APP/abuse" "$json")"
  PROOF_APPLY_WAIT=${PROOF_APPLY_WAIT:-6}
  [ "$(tail -n1 <<<"$out")" = 200 ] || { fail "PUT abuse $json: $(body_of <<<"$out")"; exit 1; }
  sleep "$PROOF_APPLY_WAIT"
  for _ in $(seq 1 40); do
    reason="$(curl -sS -m 20 -H "Authorization: Bearer $LEVELRAIL_TOKEN" "$API/api/v1/apps/$APP/abuse" | jq -r '.condition.reason')"
    [ "$reason" = "$want" ] && return 0
    sleep 1
  done
  fail "policy never reached $want (last $reason)"
  exit 1
}

# rotate N: the mode order shifted N-1 places, so no mode always runs first.
rotate() { case "$1" in 1) echo off log_only enforce ;; 2) echo log_only enforce off ;; *) echo enforce off log_only ;; esac; }

# status URL [extra curl args]: HTTP code only.
status() { local u="$1"; shift; curl -sS -m 15 -o /dev/null -w '%{http_code}' "$@" "$u"; }

# measure N: python keep-alive client, prints one JSON line {"lat":[ms...],"codes":{...}}.
MEASURE='
import http.client, json, ssl, sys, time
host, n = sys.argv[1], int(sys.argv[2])
ctx = ssl.create_default_context()
conn = http.client.HTTPSConnection(host, 443, context=ctx, timeout=15)
lat, codes = [], {}
for i in range(n + 10):
    t = time.perf_counter()
    try:
        conn.request("GET", "/", headers={"User-Agent": "Mozilla/5.0 proof-abuse-latency"})
        r = conn.getresponse(); r.read(); c = str(r.status)
    except Exception as e:
        c = "err"; conn.close(); conn = http.client.HTTPSConnection(host, 443, context=ctx, timeout=15)
    d = (time.perf_counter() - t) * 1000
    if i >= 10:
        lat.append(round(d, 3)); codes[c] = codes.get(c, 0) + 1
print(json.dumps({"lat": lat, "codes": codes}))
'
measure_local() { python3 -I -c "$MEASURE" "$host" "$1"; }
measure_host() { printf "%s" "$MEASURE" | ssh -o BatchMode=yes "$SSH_TARGET" python3 -I - "$host" "$1"; }

# 1. Throwaway app.
out="$(api POST /api/v1/apps "{\"name\":\"$APP\",\"image\":\"$BASE_IMAGE\",\"port\":80}")"
code="$(tail -n1 <<<"$out")"
if [ "$code" != 201 ] && [ "$code" != 200 ]; then fail "create app HTTP $code: $(body_of <<<"$out")"; exit 1; fi
APP_CREATED=1
for _ in $(seq 1 30); do
  host="$(curl -sS -m 20 -H "Authorization: Bearer $LEVELRAIL_TOKEN" "$API/api/v1/apps/$APP/auto-host" | jq -r '.hostname // empty')"
  [ -n "$host" ] && break
  sleep 2
done
[ -n "$host" ] || { fail "no automatic hostname"; exit 1; }
for _ in $(seq 1 90); do
  [ "$(status "https://$host/" 2>/dev/null || true)" = 200 ] && break
  sleep 2
done
[ "$(status "https://$host/")" = 200 ] || { fail "app never answered 200 on https://$host/"; exit 1; }
pass "app $APP serves https://$host/ (real certificate)"

# 2. Starting state: no policy.
state="$(curl -sS -m 20 -H "Authorization: Bearer $LEVELRAIL_TOKEN" "$API/api/v1/apps/$APP/abuse" | jq -r '"configured=\(.configured) reason=\(.condition.reason)"')"
log "initial abuse state: $state"

# 3. Replay: some normal traffic, then a draft policy replayed against it.
for _ in $(seq 1 30); do status "https://$host/" -A "Mozilla/5.0 proof-abuse-seed" >/dev/null; done
out="$(api POST "/api/v1/apps/$APP/abuse/dry-run" '{"minutes":15,"policy":{"mode":"enforce","rps":5,"burst":5,"deny_user_agents":["proof-abuse-seed"]}}')"
if [ "$(tail -n1 <<<"$out")" = 200 ]; then
  log "dry-run: $(body_of <<<"$out" | jq -c '{requests,would_block,percent,by_reason,covered_minutes}')"
  [ "$(body_of <<<"$out" | jq '.requests')" -ge 1 ] && pass "dry-run replayed recorded traffic" || fail "dry-run saw no requests"
else
  fail "dry-run HTTP $(tail -n1 <<<"$out"): $(body_of <<<"$out")"
fi

# 4. log_only: nothing blocked, would_block counted.
POL_BASE='"rps":5000,"burst":10000,"deny_user_agents":["proof-abuse-badbot"]'
set_policy "{\"mode\":\"log_only\",$POL_BASE}" LogOnly
c="$(status "https://$host/" -A "proof-abuse-badbot/1.0")"
[ "$c" = 200 ] && pass "log_only serves a denied User-Agent (HTTP $c)" || fail "log_only answered $c for a denied User-Agent"
sleep "${PROOF_COUNTER_WAIT:-15}"
log "log_only counters: $(curl -sS -m 20 -H "Authorization: Bearer $LEVELRAIL_TOKEN" "$API/api/v1/apps/$APP/abuse" | jq -c '.counters | {allowed,would_block,blocked,fail_open}')"

# 5. enforce: denied agent 403, normal browser session all 200, burst over the limit gets 429.
set_policy "{\"mode\":\"enforce\",$POL_BASE}" Enforcing
c="$(status "https://$host/" -A "proof-abuse-badbot/1.0")"
[ "$c" = 403 ] && pass "enforce turns away the denied User-Agent (HTTP $c)" || fail "enforce answered $c for a denied User-Agent"
bad=0
for p in / /index.html /missing-page /favicon.ico / /; do
  for _ in 1 2 3 4; do
    c="$(status "https://$host$p" -A 'Mozilla/5.0 (Macintosh; Intel Mac OS X 14_0) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0 Safari/537.36')"
    case "$c" in 200 | 404) ;; *) bad=$((bad + 1)) ;; esac
  done
done
[ "$bad" = 0 ] && pass "enforce: 24 normal browser requests, no false positive" || fail "enforce blocked $bad normal browser requests"

set_policy '{"mode":"enforce","rps":5,"burst":10}' Enforcing
burst_out="$(seq 1 60 | xargs -P 20 -I{} curl -sS -m 15 -o /dev/null -D - -A 'Mozilla/5.0 proof-abuse-burst' "https://$host/" | tr -d '\r' | grep -iE '^(HTTP|retry-after)')"
got429="$(grep -c ' 429' <<<"$burst_out" || true)"
hdr="$(grep -i '^retry-after' <<<"$burst_out" | sort -u | head -n1 || true)"
[ "$got429" -ge 1 ] && pass "rate limit rps=5 burst=10: $got429 of 60 parallel requests got 429 ($hdr)" || fail "no 429 in 60 parallel requests at rps=5"
sleep "${PROOF_COUNTER_WAIT:-15}"
log "enforce counters: $(curl -sS -m 20 -H "Authorization: Bearer $LEVELRAIL_TOKEN" "$API/api/v1/apps/$APP/abuse" | jq -c '.counters | {allowed,would_block,blocked,fail_open,by_reason}')"

# 6. Latency: interleaved rounds over off, log_only, enforce (limits high enough to block nothing).
log "latency: $ROUNDS rounds, $N_LOCAL requests per mode per round locally${SSH_TARGET:+, $N_HOST from $SSH_TARGET}"
for round in $(seq 1 "$ROUNDS"); do
  for mode in $(rotate "$round"); do
    case "$mode" in
      off) set_policy '{"mode":"off"}' Off ;;
      log_only) set_policy "{\"mode\":\"log_only\",$POL_BASE}" LogOnly ;;
      enforce) set_policy "{\"mode\":\"enforce\",$POL_BASE}" Enforcing ;;
    esac
    measure_local "$N_LOCAL" >>"$WORK/local.$mode.jsonl"
    if [ -n "$SSH_TARGET" ]; then measure_host "$N_HOST" >>"$WORK/host.$mode.jsonl"; fi
  done
done

python3 -I - "$WORK" "$([ -n "$SSH_TARGET" ] && echo yes || echo no)" <<'PY'
import json, sys, os
work, has_host = sys.argv[1], sys.argv[2] == "yes"
def pct(v, p):
    v = sorted(v); k = (len(v) - 1) * p; f = int(k); c = min(f + 1, len(v) - 1)
    return v[f] + (v[c] - v[f]) * (k - f)
for vantage in ["local"] + (["host"] if has_host else []):
    base = None
    print(f"proof-abuse: latency from {vantage} (ms, pooled across rounds)")
    for mode in ["off", "log_only", "enforce"]:
        lat, codes = [], {}
        for line in open(os.path.join(work, f"{vantage}.{mode}.jsonl")):
            d = json.loads(line); lat += d["lat"]
            for k, v in d["codes"].items(): codes[k] = codes.get(k, 0) + v
        p50, p95, p99 = pct(lat, .5), pct(lat, .95), pct(lat, .99)
        if base is None: base = p50
        print(f"proof-abuse:   {mode:9s} n={len(lat):4d} p50={p50:7.3f} p95={p95:7.3f} p99={p99:7.3f} mean={sum(lat)/len(lat):7.3f} p50_delta={p50-base:+.3f} codes={codes}")
PY
exit "$FAIL"
