#!/usr/bin/env bash
# Machine-wide admission control for heavy hook jobs (tsc, vitest, golangci-lint,
# the internal/api compile and shards). Source it, then: slot_run <name> <cmd...>
# Many agents commit at once; unbounded, their heavy checks thrash the machine.
#
# Env: LEVELRAIL_HEAVY_SLOTS    slots machine-wide (default 3)
#      LEVELRAIL_SLOT_DIR       lock root (default ${TMPDIR:-/tmp}/levelrail-slots)
#      LEVELRAIL_SLOT_MAX_WAIT  seconds before running anyway with a warning (default 1800)
#      LEVELRAIL_SLOT_NOTICE    seconds between wait lines (default 30)
#      LEVELRAIL_SLOT_POLL      seconds between slot probes (default 1)
# bash 3.2 compatible (macOS has no flock); mkdir is the atomic lock.

_slot_int() { # <value> <default>
	case "$1" in
	'' | *[!0-9]*) echo "$2" ;;
	*) if [ "$1" -ge 1 ]; then echo "$1"; else echo "$2"; fi ;;
	esac
}

# Wait lines go to fd 3 when a hook opened it (its stderr may be captured in a log).
_slot_say() {
	if { true >&3; } 2>/dev/null; then echo "$*" >&3; else echo "$*" >&2; fi
}

# Prints the claimed slot dir, or nothing when every slot is busy.
_slot_try() { # <root> <count> <owner>
	local root="$1" n="$2" owner="$3" i=1 d pid
	while [ "$i" -le "$n" ]; do
		d="$root/heavy.$i"
		if mkdir "$d" 2>/dev/null; then
			echo "$owner" >"$d/pid"
			echo "$d"
			return 0
		fi
		pid="$(cat "$d/pid" 2>/dev/null || true)"
		# An empty pid file means the owner is between mkdir and write, or died there.
		if { [ -n "$pid" ] && ! kill -0 "$pid" 2>/dev/null; } ||
			{ [ -z "$pid" ] && [ -d "$d" ] && [ -n "$(find "$d" -maxdepth 0 -mmin +1 2>/dev/null)" ]; }; then
			# Rename is atomic, so only one reclaimer wins the dead slot.
			if mv "$d" "$d.stale.$owner.$RANDOM" 2>/dev/null; then
				rm -rf "$d".stale.* 2>/dev/null || true
			fi
		fi
		i=$((i + 1))
	done
	return 1
}

_slot_busy() { # <root> <count>
	local root="$1" n="$2" i=1 c=0
	while [ "$i" -le "$n" ]; do
		[ -d "$root/heavy.$i" ] && c=$((c + 1))
		i=$((i + 1))
	done
	echo "$c"
}

slot_run() {
	local name="$1"
	shift
	# A command already inside a slot (nested call) must not take a second one.
	if [ -n "${LEVELRAIL_SLOT_HELD:-}" ]; then
		"$@"
		return $?
	fi
	(
		root="${LEVELRAIL_SLOT_DIR:-${TMPDIR:-/tmp}/levelrail-slots}"
		n="$(_slot_int "${LEVELRAIL_HEAVY_SLOTS:-}" 3)"
		max_wait="$(_slot_int "${LEVELRAIL_SLOT_MAX_WAIT:-}" 1800)"
		notice="$(_slot_int "${LEVELRAIL_SLOT_NOTICE:-}" 30)"
		poll="${LEVELRAIL_SLOT_POLL:-1}"
		mkdir -p "$root" 2>/dev/null || true
		got=""
		start="$(date +%s)"
		last=0
		while :; do
			got="$(_slot_try "$root" "$n" "$$")" && break
			got=""
			now="$(date +%s)"
			if [ $((now - start)) -ge "$max_wait" ]; then
				_slot_say "slot: waited ${max_wait}s for a build slot ($name), running anyway"
				break
			fi
			if [ "$last" -eq 0 ] || [ $((now - last)) -ge "$notice" ]; then
				_slot_say "waiting for a build slot ($(_slot_busy "$root" "$n") busy)"
				last="$now"
			fi
			sleep "$poll"
		done
		child=""
		trap '[ -z "$got" ] || rm -rf "$got"' EXIT
		trap '[ -z "$child" ] || kill "$child" 2>/dev/null; exit 130' INT
		trap '[ -z "$child" ] || kill "$child" 2>/dev/null; exit 143' TERM
		export LEVELRAIL_SLOT_HELD=1
		# Background plus wait so a TERM reaches the trap now, not after the job ends.
		"$@" <&0 &
		child=$!
		wait "$child"
		exit $?
	)
}
