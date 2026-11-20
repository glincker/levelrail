#!/usr/bin/env bash
# Real-host proof: two-node WireGuard mesh. Joins a second machine as a mesh
# node under a transient systemd unit with its own data dir (the machine's own
# installed instance is not touched), checks the tunnel, runs a throwaway app on
# the remote node behind the hub's ingress, drains the node, simulates losing
# it, then removes everything it created. Safe to re-run.
#
# Required env:
#   LEVELRAIL_API_URL   dashboard address of the hub (the control plane with the mesh on)
#   LEVELRAIL_TOKEN     root API token (short lived)
#   PROOF_NODE_SSH      ssh target of the machine to join (passwordless sudo, Docker installed)
#   PROOF_AGENT_BIN     local path of a levelrail-agent binary built for that machine
# Optional env:
#   PROOF_NODE_SSH_OPTS extra ssh options for the node, for example "-i ~/.ssh/key"
#   PROOF_HUB_SSH       ssh target of the hub, to ping the node over the tunnel from the hub side
#   PROOF_NODE_NAME     node name (default proof-mesh-node)
#   PROOF_APP           throwaway app name (default proof-mesh-app)
#   PROOF_UNIT          transient unit and directory name on the node (default proof-mesh-agent)
#   PROOF_BASE_IMAGE    image the app runs (default nginx:alpine)
set -euo pipefail

: "${LEVELRAIL_API_URL:?set LEVELRAIL_API_URL}"
: "${LEVELRAIL_TOKEN:?set LEVELRAIL_TOKEN}"
: "${PROOF_NODE_SSH:?set PROOF_NODE_SSH}"
: "${PROOF_AGENT_BIN:?set PROOF_AGENT_BIN}"
NODE_SSH_OPTS="${PROOF_NODE_SSH_OPTS:-}"
HUB_SSH="${PROOF_HUB_SSH:-}"
NODE_NAME="${PROOF_NODE_NAME:-proof-mesh-node}"
APP="${PROOF_APP:-proof-mesh-app}"
UNIT="${PROOF_UNIT:-proof-mesh-agent}"
BASE_IMAGE="${PROOF_BASE_IMAGE:-nginx:alpine}"
API="${LEVELRAIL_API_URL%/}"
WORK="$(mktemp -d)"
NODE_ID=""
APP_CREATED=0
UNIT_STARTED=0
FAIL=0

log() { printf '%s\n' "proof-mesh: $*"; }
pass() { log "PASS $*"; }
fail() { log "FAIL $*"; FAIL=1; }
now() { date +%s; }

api() {
  local method="$1" path="$2" body="${3:-}"
  if [ -n "$body" ]; then
    curl -sS -m 60 -X "$method" -H "Authorization: Bearer $LEVELRAIL_TOKEN" -H 'Content-Type: application/json' -d "$body" -w '\n%{http_code}' "$API$path"
  else
    curl -sS -m 60 -X "$method" -H "Authorization: Bearer $LEVELRAIL_TOKEN" -w '\n%{http_code}' "$API$path"
  fi
}
body_of() { sed '$d'; }
getj() { curl -sS -m 30 -H "Authorization: Bearer $LEVELRAIL_TOKEN" "$API$1"; }
# shellcheck disable=SC2086
nssh() { ssh -o BatchMode=yes -o ConnectTimeout=15 $NODE_SSH_OPTS "$PROOF_NODE_SSH" "$@"; }
# shellcheck disable=SC2086
nscp() { scp -q $NODE_SSH_OPTS "$@"; }

node_snapshot() {
  nssh 'sudo docker ps -a --format "{{.Names}}" | sort; echo ---; sudo docker images --format "{{.Repository}}:{{.Tag}} {{.ID}}" | sort; echo ---; ip -br link | cut -d" " -f1 | sort; echo ---; ss -lntu | awk "NR>1{print \$1,\$5}" | sort'
}

start_agent() {
  # The identity file keeps the node's enrollment, so a restart needs no token.
  nssh "sudo systemd-run --quiet --unit=$UNIT --description='mesh proof agent (throwaway)' -p WorkingDirectory=/var/lib/$UNIT -p EnvironmentFile=/var/lib/$UNIT/agent.env /opt/$UNIT/levelrail-agent"
}

cleanup() {
  set +e
  log "cleanup"
  if [ "$APP_CREATED" = 1 ]; then api DELETE "/api/v1/apps/$APP" >/dev/null; fi
  if [ -n "$NODE_ID" ]; then
    for _ in $(seq 1 20); do
      code="$(api DELETE "/api/v1/nodes/$NODE_ID" | tail -n1)"
      [ "$code" = 204 ] || [ "$code" = 200 ] || [ "$code" = 404 ] && break
      sleep 3
    done
    log "node delete HTTP $code"
  fi
  if [ "$UNIT_STARTED" = 1 ]; then
    nssh "sudo systemctl stop $UNIT 2>/dev/null; sudo systemctl reset-failed $UNIT 2>/dev/null; sudo rm -rf /opt/$UNIT /var/lib/$UNIT"
    nssh "sudo docker rm -f \$(sudo docker ps -aq --filter name=$APP) 2>/dev/null; sudo docker rmi $BASE_IMAGE >/dev/null 2>&1; true"
  fi
  if [ -f "$WORK/node.before" ]; then
    node_snapshot >"$WORK/node.after" 2>/dev/null
    if diff -u "$WORK/node.before" "$WORK/node.after" >"$WORK/node.diff"; then
      pass "node is back to its original state (containers, images, links, listeners)"
    else
      log "node differs from its start state:"
      cat "$WORK/node.diff"
    fi
  fi
  rm -rf "$WORK"
  exit "$FAIL"
}
trap cleanup EXIT

# 0. Preconditions and the node's start state.
mesh="$(getj /api/v1/mesh)"
[ "$(jq -r .enabled <<<"$mesh")" = true ] || { fail "the hub has the mesh off"; exit 1; }
node_snapshot >"$WORK/node.before"
log "hub mesh backend $(jq -r .backend <<<"$mesh"), interface $(jq -r .interface <<<"$mesh")"

# 1. Join token with the mesh, a data dir and a transient unit on the node.
t0="$(now)"
out="$(api POST /api/v1/nodes/join-tokens "{\"node_name\":\"$NODE_NAME\",\"mesh\":true}")"
[ "$(tail -n1 <<<"$out")" = 201 ] || [ "$(tail -n1 <<<"$out")" = 200 ] || { fail "join token HTTP $(tail -n1 <<<"$out"): $(body_of <<<"$out" | sed -E 's/"token":"[^"]*"/"token":"<redacted>"/')"; exit 1; }
tok_json="$(body_of <<<"$out")"
nssh "sudo mkdir -p /opt/$UNIT /var/lib/$UNIT && sudo chmod 700 /var/lib/$UNIT"
nscp "$PROOF_AGENT_BIN" "$PROOF_NODE_SSH:/tmp/$UNIT.bin"
nssh "sudo install -m 755 /tmp/$UNIT.bin /opt/$UNIT/levelrail-agent && rm -f /tmp/$UNIT.bin"
{
  printf 'APP_CONTROL_PLANE_ADDR=%s\n' "$(jq -r .control_plane_addr <<<"$tok_json")"
  printf 'APP_JOIN_TOKEN=%s\n' "$(jq -r .token <<<"$tok_json")"
  printf 'APP_CA_FINGERPRINT=%s\n' "$(jq -r .ca_fingerprint <<<"$tok_json")"
  printf 'APP_NODE_NAME=%s\n' "$NODE_NAME"
  printf 'APP_AGENT_IDENTITY_FILE=/var/lib/%s/identity.json\n' "$UNIT"
  printf 'APP_MESH_ENABLED=1\n'
} | nssh "sudo tee /var/lib/$UNIT/agent.env >/dev/null && sudo chmod 600 /var/lib/$UNIT/agent.env"
start_agent
UNIT_STARTED=1

node_row() { getj /api/v1/nodes | jq -c --arg n "$NODE_NAME" '[.[] | select(.name==$n)] | .[0] // empty'; }
wait_online() {
  local tries="$1"
  for _ in $(seq 1 "$tries"); do
    row="$(node_row)"
    [ -n "$row" ] && [ "$(jq -r .status <<<"$row")" = online ] && return 0
    sleep 2
  done
  return 1
}
if ! wait_online 40; then
  fail "node never came online"
  nssh "sudo journalctl -u $UNIT --no-pager 2>&1 | tail -n 12 | cut -c1-240" || true
  row="$(node_row)"
  NODE_ID="$(jq -r '.id // empty' <<<"${row:-null}")"
  exit 1
fi
NODE_ID="$(jq -r .id <<<"$row")"
# Seen on a real host: a fresh node can flip to offline about 2s after joining and stay there while its heartbeats keep arriving.
sleep 10
row="$(node_row)"
if [ "$(jq -r .status <<<"$row")" != online ]; then
  log "WARN BUG node status $(jq -r .status <<<"$row") while connected, last seen $(jq -r .last_seen_at <<<"$row")"
  log "workaround: restarting the agent once"
  nssh "sudo systemctl restart $UNIT"
  wait_online 30 || { fail "node still not online after an agent restart"; exit 1; }
fi
pass "node $NODE_NAME online in $(($(now) - t0))s, id $NODE_ID"
# The token is spent; drop it from the env file.
nssh "sudo sed -i '/^APP_JOIN_TOKEN=/d' /var/lib/$UNIT/agent.env"

# 2. Tunnel: handshake on the hub, ping both ways.
sleep 8
peer="$(getj /api/v1/mesh | jq -c --arg id "$NODE_ID" '[.peers[] | select(.node_id==$id)] | .[0]')"
if [ "$(jq -r '.healthy and .live' <<<"$peer")" = true ] && [ "$(jq -r .last_handshake_at <<<"$peer")" != null ]; then
  pass "WireGuard peer healthy, last handshake $(jq -r .last_handshake_at <<<"$peer"), endpoint $(jq -r .endpoint <<<"$peer")"
else
  fail "peer not healthy: $peer"
fi
hub_addr="$(getj /api/v1/nodes | jq -r '.[] | select(.is_local==true) | .address')"
node_addr="$(jq -r .mesh_address <<<"$peer")"
log "ping hub $hub_addr from the node: $(nssh "ping -c 5 -W 2 $hub_addr | tail -n1")"
if [ -n "$HUB_SSH" ]; then log "ping node $node_addr from the hub: $(ssh -o BatchMode=yes "$HUB_SSH" "ping -c 5 -W 2 $node_addr | tail -n1")"; fi

# 3. App on the remote node, served through the hub's ingress.
t1="$(now)"
out="$(api POST /api/v1/apps "{\"name\":\"$APP\",\"image\":\"$BASE_IMAGE\",\"port\":80,\"node_id\":\"$NODE_ID\"}")"
code="$(tail -n1 <<<"$out")"
if [ "$code" != 201 ] && [ "$code" != 200 ]; then fail "create app HTTP $code: $(body_of <<<"$out")"; exit 1; fi
APP_CREATED=1
host=""
for _ in $(seq 1 30); do
  host="$(getj "/api/v1/apps/$APP/auto-host" | jq -r '.hostname // empty')"
  [ -n "$host" ] && break
  sleep 2
done
c=000
for _ in $(seq 1 60); do
  c="$(curl -sS -m 10 -o /dev/null -w '%{http_code}' "https://$host/" 2>/dev/null || true)"
  [ "$c" = 200 ] && break
  sleep 3
done
[ "$c" = 200 ] && pass "app on the remote node answers on https://$host/ $(($(now) - t1))s after create" || { fail "app never answered (last $c)"; exit 1; }
log "app container on the node: $(nssh "sudo docker ps --format '{{.Names}} {{.Image}} {{.Status}}' | grep $APP" || true)"
log "node-side access log shows the hub as peer: $(nssh "sudo docker logs \$(sudo docker ps -q --filter name=$APP | head -n1) 2>&1 | tail -n1 | cut -c1-120" || true)"
log "request time through ingress and tunnel: $(for _ in 1 2 3; do curl -sS -m 10 -o /dev/null -w '%{time_total}s ' "https://$host/"; done)"

# 4. Drain: the app moves to the hub with requests running.
: >"$WORK/loop.log"
(while [ ! -f "$WORK/stop" ]; do printf '%s\n' "$(curl -sS -m 5 -o /dev/null -w '%{http_code}' "https://$host/" 2>/dev/null || echo 000)" >>"$WORK/loop.log"; sleep 0.5; done) &
loop=$!
sleep 3
t2="$(now)"
out="$(api POST "/api/v1/nodes/$NODE_ID/drain")"
log "drain request HTTP $(tail -n1 <<<"$out")"
st=""
for _ in $(seq 1 60); do
  st="$(getj "/api/v1/nodes/$NODE_ID/drains" | jq -r '.[0].status // empty')"
  case "$st" in succeeded | failed | partial) break ;; esac
  sleep 2
done
t3="$(now)"
sleep 4
touch "$WORK/stop"
wait "$loop" 2>/dev/null || true
total="$(wc -l <"$WORK/loop.log" | tr -d ' ')"
ok="$(grep -c '^200$' "$WORK/loop.log" || true)"
[ "$st" = succeeded ] && pass "drain $st in $((t3 - t2))s" || fail "drain status '$st'"
moved_to="$(getj "/api/v1/apps/$APP" | jq -r '.node_id // ""')"
[ "$moved_to" != "$NODE_ID" ] && pass "app now placed on ${moved_to:-the hub}" || fail "app still on the drained node"
log "requests during the drain: $ok of $total answered 200 (others: $(grep -v '^200$' "$WORK/loop.log" | sort | uniq -c | tr -s ' ' | tr '\n' ';'))"
log "node resources after drain: $(getj "/api/v1/nodes/$NODE_ID" | jq -c '{status,schedulable,resource_count}')"
api POST "/api/v1/nodes/$NODE_ID/uncordon" >/dev/null

# 5. Node loss: put the app back, stop the agent, record, bring it back.
api PUT "/api/v1/apps/$APP/node" "{\"node_id\":\"$NODE_ID\"}" >/dev/null
for _ in $(seq 1 40); do
  [ "$(curl -sS -m 5 -o /dev/null -w '%{http_code}' "https://$host/" 2>/dev/null || true)" = 200 ] && break
  sleep 2
done
t4="$(now)"
nssh "sudo systemctl stop $UNIT"
sleep 15
st="$(getj /api/v1/nodes | jq -r --arg id "$NODE_ID" '.[] | select(.id==$id) | .status')"
log "node loss after $(($(now) - t4))s: node status $st, app container on the node: $(nssh "sudo docker ps --format '{{.Status}}' --filter name=$APP" || true), https now $(curl -sS -m 5 -o /dev/null -w '%{http_code}' "https://$host/" 2>&1 | tail -c 60)"
[ "$st" = offline ] && pass "control plane marks the lost node offline" || fail "node status $st after the agent stopped"
t5="$(now)"
start_agent
c=000
for _ in $(seq 1 40); do
  c="$(curl -sS -m 5 -o /dev/null -w '%{http_code}' "https://$host/" 2>/dev/null || true)"
  [ "$c" = 200 ] && break
  sleep 2
done
[ "$c" = 200 ] && pass "agent back, app answering again $(($(now) - t5))s after the restart" || fail "app did not come back (last $c)"
log "total proof time $(($(now) - t0))s"
exit "$FAIL"
