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

# Two reclaimers of one dead slot must not move a live owner's dir away.
sleep 30 &
live=$!
for _ in 1 2 3 4 5 6 7 8 9 10; do
	rm -rf "$tmp/race"
	mkdir -p "$tmp/race/heavy.1"
	sh -c 'echo $$' >"$tmp/race/heavy.1/pid"
	rp=""
	for who in a b; do
		(for _ in 1 2 3 4; do _slot_try "$tmp/race" 1 "$live" && break; done >"$tmp/claim.$who") 2>/dev/null &
		rp="$rp $!"
	done
	# shellcheck disable=SC2086
	wait $rp
	claims="$(cat "$tmp/claim.a" "$tmp/claim.b" | grep -c heavy)"
	check test "$claims" -eq 1
	check test "$(cat "$tmp/race/heavy.1/pid" 2>/dev/null)" = "$live"
done
kill "$live" 2>/dev/null
check test ! -d "$tmp/race/reclaim.lock"

# The wrapped command's exit code is the result, and the slot is released.
slot_run rc sh -c 'exit 7' 2>/dev/null
check test $? -eq 7
check test ! -d "$tmp/slots/heavy.1"

[ "$fail" -eq 0 ] && echo "test-slot: ok"
exit "$fail"
