#!/usr/bin/env bash
# Rebuilds the demo data in a fresh directory, then swaps it in with a short
# service stop so visitors never see a half-written database.
set -euo pipefail

STATE_DIR="${DEMO_STATE_DIR:-/var/lib/levelrail-demo}"
BIN="${DEMO_BIN:-/opt/levelrail-demo/bin/levelrail}"
SEED="${DEMO_SEED:-1}"
UNIT="levelrail-demo.service"
RUN_USER="levelrail-demo"

data="$STATE_DIR/data"
next="$STATE_DIR/data.next"
old="$STATE_DIR/data.old"

rm -rf "$next" "$old"
install -d -m 0700 -o "$RUN_USER" -g "$RUN_USER" "$next"
runuser -u "$RUN_USER" -- env APP_DATA_DIR="$next" "$BIN" demo seed --seed "$SEED"

systemctl stop "$UNIT"
if [ -d "$data" ]; then mv "$data" "$old"; fi
mv "$next" "$data"
systemctl start "$UNIT"
rm -rf "$old"
echo "reseeded"
