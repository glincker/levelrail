#!/usr/bin/env bash
# Host level chaos run for an already installed test host: stop the container
# runtime, then fill a disk, recording how the control plane and an app behave
# and whether each fault converges once removed. Faults only ever touch a
# marker file and the runtime service; needs PROOF_CONFIRM=chaos (owner approval).
#
# Required env: PROOF_HOST ([user@]host with root or passwordless sudo),
#   PROOF_APP_URL (app URL that returns 200), PROOF_CP_URL (control plane base URL)
# Optional env: PROOF_SSH_OPTS, PROOF_FILL_DIR (default /var/lib/levelrail),
#   PROOF_FILL_FREE_MB (free space to leave, default 512), PROOF_RECOVER_TIMEOUT (default 300),
#   LEVELRAIL_PROOF_OUT
# Usage: scripts/proof-chaos-host.sh [--dry-run]
set -euo pipefail

host="${PROOF_HOST:?set PROOF_HOST}"
app_url="${PROOF_APP_URL:?set PROOF_APP_URL}"
cp_url="${PROOF_CP_URL:?set PROOF_CP_URL}"
fill_dir="${PROOF_FILL_DIR:-/var/lib/levelrail}"
free_mb="${PROOF_FILL_FREE_MB:-512}"
recover_s="${PROOF_RECOVER_TIMEOUT:-300}"
out_dir="${LEVELRAIL_PROOF_OUT:-${TMPDIR:-/tmp}/levelrail-proof}"
# shellcheck disable=SC2206
ssh_opts=(-o BatchMode=yes -o ConnectTimeout=8 ${PROOF_SSH_OPTS:-})

dry=0
[ "${1:-}" = "--dry-run" ] && dry=1
if [ "$dry" = 1 ]; then
	echo "would stop docker on $host, record, restart, then fill $fill_dir to ${free_mb} MB free, record, remove the marker file"
	exit 0
fi
if [ "${PROOF_CONFIRM:-}" != "chaos" ]; then
	echo "proof-chaos-host: refusing to run on $host without PROOF_CONFIRM=chaos" >&2
	exit 2
fi

mkdir -p "$out_dir"
log="$out_dir/chaos-host.tsv"
note() { printf '%s\t%s\t%s\n' "$(date -u +%FT%TZ)" "$1" "$2" | tee -a "$log"; }
code() { curl -s -o /dev/null -w '%{http_code}' --max-time 8 "$1" || true; }
remote() { ssh "${ssh_opts[@]}" "$host" "$@"; }
marker="$fill_dir/.proof-chaos-fill"

cleanup() {
	remote "sudo rm -f '$marker'; sudo systemctl start docker.socket docker" >/dev/null 2>&1 || true
}
trap cleanup EXIT

wait_for() {
	local label="$1" url="$2" start
	start="$(date +%s)"
	while [ $(($(date +%s) - start)) -lt "$recover_s" ]; do
		if [ "$(code "$url")" = 200 ]; then
			note "$label" "converged after $(($(date +%s) - start))s"
			return 0
		fi
		sleep 3
	done
	note "$label" "no recovery in ${recover_s}s"
	return 1
}

[ "$(code "$cp_url/healthz")" = 200 ] && [ "$(code "$app_url")" = 200 ] || { note precheck "control plane or app not healthy"; exit 1; }
note precheck "healthy"
rc=0

remote 'sudo systemctl stop docker.socket docker'
sleep 10
note dockerd-stop "control plane $(code "$cp_url/healthz"), app $(code "$app_url")"
remote 'sudo systemctl start docker.socket docker'
wait_for dockerd-restore-app "$app_url" || rc=1
wait_for dockerd-restore-cp "$cp_url/healthz" || rc=1

avail_mb="$(remote "df -Pm '$fill_dir' | awk 'NR==2{print \$4}'")"
fill_mb=$((avail_mb - free_mb))
if [ "$fill_mb" -gt 0 ]; then
	remote "sudo fallocate -l ${fill_mb}M '$marker'"
	sleep 60
	note disk-fill "left ${free_mb} MB, control plane $(code "$cp_url/healthz"), app $(code "$app_url")"
	remote "sudo rm -f '$marker'"
	wait_for disk-release-cp "$cp_url/healthz" || rc=1
else
	note disk-fill "skipped, only ${avail_mb} MB free under $fill_dir"
fi

[ "$rc" = 0 ] && note result PASS || note result FAIL
exit "$rc"
