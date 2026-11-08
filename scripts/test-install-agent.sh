#!/bin/sh
# Dry-run checks for install-agent.sh: nothing is installed, no root needed.
set -eu

here="$(cd "$(dirname "$0")" && pwd)"
script="$here/install-agent.sh"
unit="$here/../deploy/packaging/systemd/levelrail-agent-standalone.service"
fail=0

check() {
	if ! printf '%s' "$2" | grep -q -- "$3"; then
		printf 'FAIL %s: missing %s\n' "$1" "$3" >&2
		fail=1
	fi
}

out="$(printf 'tok_abc123' | sh "$script" --dry-run --server https://cp.example.com --ca-fingerprint ab12 --token-stdin --name w2 2>&1)"
check install "$out" 'write /etc/levelrail-agent/agent.env (mode 0600)'
check install "$out" 'systemctl restart levelrail-agent'
check install "$out" 'cp.example.com'
check install "$out" 'checksum'
if printf '%s' "$out" | grep -q 'tok_abc123'; then
	printf 'FAIL the token leaked into dry-run output\n' >&2
	fail=1
fi

mesh="$(printf 'tok_abc123' | sh "$script" --dry-run --server cp.example.com:9443 --token-stdin --mesh 2>&1)"
check mesh "$mesh" 'AmbientCapabilities=CAP_NET_ADMIN'
check mesh "$mesh" 'DeviceAllow=/dev/net/tun rw'

un="$(sh "$script" --dry-run --uninstall 2>&1)"
check uninstall "$un" 'Docker data and other software stay untouched'
if printf '%s' "$un" | grep -q 'docker rm\|docker volume'; then
	printf 'FAIL uninstall touches docker\n' >&2
	fail=1
fi

if sh "$script" --dry-run --token-stdin </dev/null >/dev/null 2>&1; then
	printf 'FAIL a missing --server must fail\n' >&2
	fail=1
fi

if [ "$(sh "$script" --print-unit)" != "$(cat "$unit")" ]; then
	printf 'FAIL the packaged unit drifted from the one the script writes\n' >&2
	fail=1
fi

if ! sh "$script" --print-unit --unit-name levelrail-agent-b | grep -q '^StateDirectory=levelrail-agent-b$'; then
	printf 'FAIL --unit-name must change the unit state directory\n' >&2
	fail=1
fi

[ "$fail" -eq 0 ] && echo "install-agent tests passed"
exit "$fail"
