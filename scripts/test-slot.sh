#!/usr/bin/env bash
# Self test for scripts/lib/slot.sh: exclusion with one slot, stale reclaim,
# exit code passthrough. Takes a few seconds, touches only a temp dir.
set -uo pipefail
cd "$(dirname "$0")/.." || exit 2
# shellcheck source=scripts/lib/slot.sh
. scripts/lib/slot.sh

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT
export LEVELRAIL_SLOT_DIR="$tmp/slots" LEVELRAIL_HEAVY_SLOTS=1 LEVELRAIL_SLOT_POLL=0.2 LEVELRAIL_SLOT_NOTICE=1
fail=0
check() { if ! "$@"; then echo "FAIL: $*"; fail=1; fi; }

# shellcheck disable=SC2016 # $1 is for the inner sh
slot_run first sh -c 'echo start >>"$1"; sleep 2; echo end >>"$1"' _ "$tmp/order" 2>"$tmp/err1" &
p1=$!
sleep 0.5
# shellcheck disable=SC2016
slot_run second sh -c 'echo start >>"$1"; echo end >>"$1"' _ "$tmp/order" 2>"$tmp/err2" &
p2=$!
wait "$p1"
check test $? -eq 0
wait "$p2"
check test $? -eq 0
check test "$(paste -sd, "$tmp/order")" = "start,end,start,end"
check grep -q "waiting for a build slot (1 busy)" "$tmp/err2"

# A slot owned by a dead PID is reclaimed.
mkdir -p "$tmp/slots/heavy.1"
sh -c 'echo $$' >"$tmp/slots/heavy.1/pid"
slot_run stale true 2>/dev/null
check test $? -eq 0
check test ! -d "$tmp/slots/heavy.1"

# The wrapped command's exit code is the result, and the slot is released.
slot_run rc sh -c 'exit 7' 2>/dev/null
check test $? -eq 7
check test ! -d "$tmp/slots/heavy.1"

[ "$fail" -eq 0 ] && echo "test-slot: ok"
exit "$fail"
