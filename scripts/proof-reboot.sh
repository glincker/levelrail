#!/usr/bin/env bash
# Reboot survival proof for an already installed host: an app answers, the
# host reboots, the control plane and the app answer again without a manual step.
# Reboots a real machine, so it needs PROOF_CONFIRM=reboot (the owner's approval).
#
# Required env: PROOF_HOST ([user@]host with root or passwordless sudo),
#   PROOF_APP_URL (an app URL that returns 200), PROOF_CP_URL (control plane base URL)
# Optional env: PROOF_SSH_OPTS, PROOF_UNIT (default levelrail),
#   PROOF_REBOOT_TIMEOUT (seconds to come back, default 600), LEVELRAIL_PROOF_OUT
# Usage: scripts/proof-reboot.sh [--dry-run]
set -euo pipefail

host="${PROOF_HOST:?set PROOF_HOST}"
app_url="${PROOF_APP_URL:?set PROOF_APP_URL}"
cp_url="${PROOF_CP_URL:?set PROOF_CP_URL}"
unit="${PROOF_UNIT:-levelrail}"
timeout_s="${PROOF_REBOOT_TIMEOUT:-600}"
out_dir="${LEVELRAIL_PROOF_OUT:-${TMPDIR:-/tmp}/levelrail-proof}"
# shellcheck disable=SC2206
ssh_opts=(-o BatchMode=yes -o ConnectTimeout=8 ${PROOF_SSH_OPTS:-})

dry=0
[ "${1:-}" = "--dry-run" ] && dry=1
if [ "$dry" = 0 ] && [ "${PROOF_CONFIRM:-}" != "reboot" ]; then
	echo "proof-reboot: refusing to reboot $host without PROOF_CONFIRM=reboot" >&2
	exit 2
fi

mkdir -p "$out_dir"
log="$out_dir/reboot.tsv"
note() { printf '%s\t%s\t%s\n' "$(date -u +%FT%TZ)" "$1" "$2" | tee -a "$log"; }
code() { curl -s -o /dev/null -w '%{http_code}' --max-time 8 "$1" || true; }
remote() { ssh "${ssh_opts[@]}" "$host" "$@"; }

if [ "$dry" = 1 ]; then
	echo "would check $app_url and $cp_url/healthz, reboot $host, wait up to ${timeout_s}s, recheck, then verify $unit is enabled"
	exit 0
fi

[ "$(code "$app_url")" = 200 ] || { note precheck "app not 200, aborting"; exit 1; }
remote "systemctl is-enabled $unit" >/dev/null || { note precheck "$unit is not enabled at boot"; exit 1; }
boot_before="$(remote 'cat /proc/sys/kernel/random/boot_id')"
note precheck "app 200, $unit enabled"

remote 'sudo systemctl reboot' || true
start="$(date +%s)"
until [ "$(remote 'cat /proc/sys/kernel/random/boot_id' 2>/dev/null || true)" != "$boot_before" ] &&
	[ -n "$(remote 'cat /proc/sys/kernel/random/boot_id' 2>/dev/null || true)" ]; do
	[ $(($(date +%s) - start)) -lt "$timeout_s" ] || { note reboot "host did not return in ${timeout_s}s"; exit 1; }
	sleep 5
done
note reboot "host back after $(($(date +%s) - start))s"

cp_ok=""
app_ok=""
while [ $(($(date +%s) - start)) -lt "$timeout_s" ]; do
	[ -z "$cp_ok" ] && [ "$(code "$cp_url/healthz")" = 200 ] && cp_ok=$(($(date +%s) - start)) && note recover "control plane healthy at ${cp_ok}s"
	[ -z "$app_ok" ] && [ "$(code "$app_url")" = 200 ] && app_ok=$(($(date +%s) - start)) && note recover "app answering at ${app_ok}s"
	[ -n "$cp_ok" ] && [ -n "$app_ok" ] && break
	sleep 5
done
[ -n "$cp_ok" ] && [ -n "$app_ok" ] || { note result "FAIL control plane '${cp_ok:-no}', app '${app_ok:-no}'"; exit 1; }
note result "PASS app back ${app_ok}s after the reboot started"
