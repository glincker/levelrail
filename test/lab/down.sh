#!/usr/bin/env bash
# Stops the lab control plane. Pass --purge to also delete the lab data dir.
set -euo pipefail
# shellcheck source=lib.sh
. "$(dirname "${BASH_SOURCE[0]}")/lib.sh"

lab_guard

stop_pid() {
	local pid="$1" _
	kill -0 "$pid" 2>/dev/null || return 0
	kill "$pid" 2>/dev/null || true
	for _ in $(seq 1 40); do
		kill -0 "$pid" 2>/dev/null || return 0
		sleep 0.5
	done
	kill -9 "$pid" 2>/dev/null || true
}

if [ -f "$LAB_DIR/pid" ]; then
	stop_pid "$(cat "$LAB_DIR/pid")"
	rm -f "$LAB_DIR/pid"
fi
echo "lab: stopped"

if [ "${1:-}" = "--purge" ]; then
	[ -f "$LAB_DIR/.lab" ] && rm -rf "$LAB_DIR"
	echo "lab: purged $LAB_DIR"
fi
