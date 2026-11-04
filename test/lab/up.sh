#!/usr/bin/env bash
# Starts a second control plane for the lab: its own port and data dir, nodes
# seeded straight into the store, no agents. Opt in only, not wired into CI.
#   test/lab/up.sh
# Env: LAB_DIR, LAB_PORT, LAB_NODES, LAB_ZONES, LAB_READY_TIMEOUT (see lib.sh).
set -euo pipefail
# shellcheck source=lib.sh
. "$(dirname "${BASH_SOURCE[0]}")/lib.sh"

lab_guard
if [ -n "$(lab_listening "$LAB_PORT")" ]; then
	echo "lab: port $LAB_PORT is already in use, run test/lab/down.sh or pick another LAB_PORT" >&2
	exit 1
fi

mkdir -p "$LAB_DIR/bin" "$LAB_DIR/data"
touch "$LAB_DIR/.lab"
cd "$lab_root"
go build -buildvcs=false -o "$LAB_DIR/bin/levelrail" ./cmd/levelrail
go build -buildvcs=false -o "$LAB_DIR/bin/levelrail-cli" ./cmd/levelrail-cli
go build -buildvcs=false -o "$LAB_DIR/bin/seed" ./test/lab/seed

# A fresh random root token per lab; the dev-mode fixtures file is the headless way to seed one.
if [ ! -f "$LAB_DIR/token" ]; then
	(umask 077 && printf 'lab-%s\n' "$(od -An -N16 -tx1 /dev/urandom | tr -d ' \n')" >"$LAB_DIR/token")
fi
(umask 077 && printf 'tokens:\n  - name: lab-root\n    plaintext: %s\n    abilities: [root]\n' "$(lab_token)" >"$LAB_DIR/fixtures.yml")

"$LAB_DIR/bin/seed" --data-dir "$LAB_DIR/data" --nodes "$LAB_NODES" --zones "$LAB_ZONES"

# Seeded nodes never heartbeat, so the stale timeout is stretched to keep them online.
(
	cd "$LAB_DIR"
	APP_EXPERIMENTAL=iac APP_DEV_MODE=1 APP_DEV_FIXTURES_FILE="$LAB_DIR/fixtures.yml" \
		APP_DATA_DIR="$LAB_DIR/data" APP_HTTP_ADDR="127.0.0.1:$LAB_PORT" APP_AGENT_ADDR="127.0.0.1:0" \
		APP_INGRESS_HTTP_ADDR="127.0.0.1:0" APP_INGRESS_HTTPS_ADDR="127.0.0.1:0" \
		APP_NODE_HEARTBEAT_TIMEOUT="${LAB_HEARTBEAT_TIMEOUT:-8760h}" \
		APP_WATCH_POLL_INTERVAL="${LAB_WATCH_POLL_INTERVAL:-200ms}" \
		nohup "$LAB_DIR/bin/levelrail" >"$LAB_DIR/server.log" 2>&1 &
	echo $! >"$LAB_DIR/pid"
)

for _ in $(seq 1 $((LAB_READY_TIMEOUT * 2))); do
	if curl -fsS -o /dev/null "$LAB_URL/api/v1/brand" 2>/dev/null; then
		echo "lab: up at $LAB_URL, data $LAB_DIR/data, $LAB_NODES nodes, token in $LAB_DIR/token"
		exit 0
	fi
	sleep 0.5
done
echo "lab: control plane did not come up, see $LAB_DIR/server.log" >&2
tail -10 "$LAB_DIR/server.log" >&2 || true
exit 1
