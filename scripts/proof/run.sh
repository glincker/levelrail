#!/usr/bin/env bash
# Runs the proof steps in order: metrics, capabilities, transcripts, shots,
# then posts one sticky comment per --pr/--issue target. Not installed in any
# hook; call it by hand, from CI, or from your own hook.
# Flags: --no-shots --strict --build --dry-run --pr N --issue N (repeatable)
set -euo pipefail
# shellcheck source=lib.sh
. "$(dirname "${BASH_SOURCE[0]}")/lib.sh"

# Whole run is capped at 5 minutes and runs at low priority.
if [ -z "${LEVELRAIL_PROOF_INNER:-}" ]; then
  # shellcheck disable=SC2016
  exec env LEVELRAIL_PROOF_INNER=1 "$BASH" -c 'self=$1; shift; t=$(command -v timeout || command -v gtimeout || true); if [ -n "$t" ]; then exec "$t" "${LEVELRAIL_PROOF_RUN_TIMEOUT:-300}" nice -n 15 "$BASH" "$self" "$@"; else exec nice -n 15 "$BASH" "$self" "$@"; fi' _ "${BASH_SOURCE[0]}" "$@"
fi

shots=1 strict=0 build=0 dry=0
targets=()
while [ $# -gt 0 ]; do
  case "$1" in
    --no-shots) shots=0 ;;
    --strict) strict=1 ;;
    --build) build=1 ;;
    --dry-run) dry=1 ;;
    --pr) targets+=("pr:$2"); shift ;;
    --issue) targets+=("issue:$2"); shift ;;
    *) echo "run.sh: unknown flag $1" >&2; exit 2 ;;
  esac
  shift
done

mkdir -p "$PROOF_OUT"
rm -f "$PROOF_OUT"/*.md

if [ "$build" = 1 ]; then
  echo "run.sh: --build requested, building both binaries"
  (cd "$PROOF_ROOT" && nice -n 15 go build -buildvcs=false -o ./levelrail ./cmd/levelrail \
    && nice -n 15 go build -buildvcs=false -o ./levelrail-cli ./cmd/levelrail-cli)
fi

# Preflight: skip only the shots, with the reason, when the machine is busy.
skip_reason=""
if [ "$shots" = 1 ]; then
  cores="$(getconf _NPROCESSORS_ONLN 2>/dev/null || sysctl -n hw.ncpu 2>/dev/null || echo 1)"
  load="$(uptime | sed -E 's/.*load averages?: *([0-9.]+).*/\1/')"
  max_load="${LEVELRAIL_PROOF_MAX_LOAD:-$cores}"
  min_disk="${LEVELRAIL_PROOF_MIN_DISK_GB:-10}"
  min_mem_mb="${LEVELRAIL_PROOF_MIN_MEM_MB:-1024}"
  free_gb="$(( $(df -Pk "${TMPDIR:-/tmp}" | awk 'NR==2{print $4}') / 1048576 ))"
  if awk -v l="$load" -v m="$max_load" 'BEGIN{exit !(l>m)}'; then
    skip_reason="1-minute load $load is above $max_load"
  elif [ "$free_gb" -lt "$min_disk" ]; then
    skip_reason="free disk ${free_gb} GB is below ${min_disk} GB"
  elif [ -r /proc/meminfo ]; then
    avail_mb="$(awk '/MemAvailable/{print int($2/1024)}' /proc/meminfo)"
    [ "$avail_mb" -lt "$min_mem_mb" ] && skip_reason="free memory ${avail_mb} MB is below ${min_mem_mb} MB"
  elif command -v vm_stat >/dev/null 2>&1; then
    avail_mb="$(vm_stat | awk '/Pages free|Pages inactive/{gsub("\\.","",$3); s+=$3} END{print int(s*4096/1048576)}')"
    [ "$avail_mb" -lt "$min_mem_mb" ] && skip_reason="free memory ${avail_mb} MB is below ${min_mem_mb} MB"
  fi
  if [ -n "$skip_reason" ]; then
    echo "run.sh: skipping screenshots: $skip_reason"
    shots=0
  fi
fi

"$PROOF_ROOT/scripts/proof/metrics.sh" >"$PROOF_OUT/metrics.md"
cap_flag=()
[ "$strict" = 1 ] && cap_flag=(--strict)
cap_rc=0
"$PROOF_ROOT/scripts/proof/capabilities.sh" "${cap_flag[@]}" >"$PROOF_OUT/capabilities.md" || cap_rc=$?
"$PROOF_ROOT/scripts/proof/transcript.sh" >"$PROOF_OUT/transcripts.md"

rm -rf "$PROOF_OUT/shots"
if [ "$shots" = 1 ]; then
  "$PROOF_ROOT/scripts/proof/shots.sh" >"$PROOF_OUT/shots.log" 2>&1 || echo "run.sh: shots step failed, see $PROOF_OUT/shots.log" >&2
  cat "$PROOF_OUT/shots.log"
fi

for t in "${targets[@]}"; do
  args=("${t%%:*}" "${t#*:}")
  [ "$dry" = 1 ] && args+=(--dry-run)
  "$PROOF_ROOT/scripts/proof/comment.sh" "${args[@]}"
done
[ "$dry" = 1 ] && [ "${#targets[@]}" -eq 0 ] && cat "$PROOF_OUT/metrics.md" "$PROOF_OUT/capabilities.md"

exit "$cap_rc"
