#!/bin/sh
# Runs install-cli.sh end to end inside a plain (non-systemd) container:
#   scripts/test-install-cli.sh <base-image> <path-to-cli-binary>
# e.g. scripts/test-install-cli.sh ubuntu:24.04 ./levelrail-cli-linux-amd64
# No --privileged needed: install-cli.sh has no service/systemd component,
# unlike install.sh's own test in scripts/test-install-sh.sh.
set -eu

base_image="$1"
binary="$2"
repo_root="$(cd "$(dirname "$0")/.." && pwd)"
name="install-cli-test-$$"

cleanup() { docker rm -f "$name" >/dev/null 2>&1 || true; }
trap cleanup EXIT

docker run -d --name "$name" "$base_image" sleep infinity >/dev/null
in_ct() { docker exec "$name" sh -c "$1"; }
in_ct 'command -v curl >/dev/null 2>&1' || in_ct 'apt-get update -qq && apt-get install -y -qq --no-install-recommends curl ca-certificates python3 >/dev/null'

docker cp "$repo_root/install-cli.sh" "$name:/root/install-cli.sh"
docker cp "$binary" "$name:/root/levelrail-cli"

echo "== install"
in_ct 'LEVELRAIL_BINARY_FILE=/root/levelrail-cli sh /root/install-cli.sh'
in_ct 'test -x /root/.local/bin/levelrail-cli' || { echo "binary not installed where expected"; exit 1; }

echo "== re-run is idempotent"
in_ct 'LEVELRAIL_BINARY_FILE=/root/levelrail-cli sh /root/install-cli.sh'

echo "== release verification (mirror, no cosign in the container)"
in_ct 'command -v cosign' >/dev/null 2>&1 && { echo "test expects no cosign in the container"; exit 1; }
# shellcheck disable=SC2016 # runs inside the container, expands there
in_ct 'goarch=amd64; [ "$(uname -m)" = x86_64 ] || goarch=arm64
	asset=levelrail-cli-linux-$goarch
	mkdir -p /srv/rel/v9.9.9/good /srv/rel/v9.9.9/tampered
	for d in good tampered; do printf "#!/bin/sh\nexit 1\n" > /srv/rel/v9.9.9/$d/$asset; done
	(cd /srv/rel/v9.9.9/good && sha256sum "$asset" > checksums.txt)
	printf "%064d  %s\n" 0 "$asset" > /srv/rel/v9.9.9/tampered/checksums.txt
	nohup python3 -m http.server 8099 --bind 127.0.0.1 --directory /srv/rel >/dev/null 2>&1 &
	sleep 1'

mirror_install() {
	dir="$1"
	shift
	docker exec "$name" env LEVELRAIL_VERSION=v9.9.9 LEVELRAIL_RELEASE_BASE_URL="http://127.0.0.1:8099/v9.9.9/$dir" \
		LEVELRAIL_INSECURE_MIRROR=1 "$@" sh /root/install-cli.sh 2>&1 || true
}

out="$(mirror_install tampered)"
echo "$out" | grep -q "checksum mismatch" || { echo "tampered checksums.txt was not rejected: $out"; exit 1; }

out="$(mirror_install good APP_INSTALL_VERIFY=require)"
echo "$out" | grep -q "APP_INSTALL_VERIFY=require but cosign is not installed" ||
	{ echo "require mode without cosign did not fail closed: $out"; exit 1; }

out="$(mirror_install good)"
echo "$out" | grep -q "Checksum verified" || { echo "good checksum was not accepted in auto mode: $out"; exit 1; }

echo "install-cli.sh passed on ${base_image}"
