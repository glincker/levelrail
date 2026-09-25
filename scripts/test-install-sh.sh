#!/bin/sh
# Runs install.sh end to end inside a systemd-enabled container:
#   scripts/test-install-sh.sh <base-image> <path-to-linux-binary>
# e.g. scripts/test-install-sh.sh ubuntu:24.04 ./levelrail-linux-amd64
# Needs a Docker daemon that can run --privileged containers.
set -eu

base_image="$1"
binary="$2"
repo_root="$(cd "$(dirname "$0")/.." && pwd)"
tag="install-sh-test:$(printf '%s' "$base_image" | tr ':/' '--')"
name="install-sh-test-$$"
wait_secs="${INSTALL_TEST_WAIT:-900}"

cleanup() { docker rm -f "$name" >/dev/null 2>&1 || true; }
trap cleanup EXIT

docker build -q -t "$tag" - >/dev/null <<EOF
FROM ${base_image}
ENV DEBIAN_FRONTEND=noninteractive
RUN apt-get update -qq && apt-get install -y -qq --no-install-recommends \
      systemd systemd-sysv dbus curl ca-certificates iproute2 procps python3 >/dev/null \
 && rm -rf /var/lib/apt/lists/* \
 && systemctl mask getty.target console-getty.service systemd-logind.service
STOPSIGNAL SIGRTMIN+3
CMD ["/sbin/init"]
EOF

docker run -d --name "$name" --privileged --cgroupns=host \
	-v /sys/fs/cgroup:/sys/fs/cgroup:rw --tmpfs /run --tmpfs /run/lock \
	-v /var/lib/docker "$tag" >/dev/null

in_ct() { docker exec "$name" sh -c "$1"; }

tries=0
until in_ct 'systemctl is-system-running 2>/dev/null | grep -Eq "running|degraded"'; do
	tries=$((tries + 1))
	[ "$tries" -lt 60 ] || { echo "systemd did not boot"; exit 1; }
	sleep 1
done

docker cp "$repo_root/install.sh" "$name:/root/install.sh"
docker cp "$binary" "$name:/root/levelrail"

echo "== install"
docker exec "$name" timeout "$wait_secs" env \
	LEVELRAIL_BINARY_FILE=/root/levelrail LEVELRAIL_PUBLIC_IP=127.0.0.1 LEVELRAIL_MIN_DISK_GB=1 \
	sh /root/install.sh

echo "== assertions"
in_ct 'curl -fsS --max-time 5 http://127.0.0.1:8080/healthz'
echo
status="$(in_ct 'curl -fsS --max-time 5 http://127.0.0.1:8080/api/v1/auth/setup-status')"
echo "setup-status: $status"
echo "$status" | grep -q '"needs_setup":true' || { echo "expected needs_setup=true"; exit 1; }
perms="$(in_ct 'stat -c %a /var/lib/levelrail-data/setup-token')"
[ "$perms" = "600" ] || { echo "setup-token perms $perms, want 600"; exit 1; }
in_ct 'APP_DATA_DIR=/var/lib/levelrail-data /usr/local/bin/levelrail setup-token' | grep -qF -- "$(in_ct 'cat /var/lib/levelrail-data/setup-token')" ||
	{ echo "setup-token subcommand did not print the token file's token"; exit 1; }

echo "== re-run install is idempotent"
docker exec "$name" timeout "$wait_secs" env \
	LEVELRAIL_BINARY_FILE=/root/levelrail LEVELRAIL_SKIP_REACHABILITY=1 LEVELRAIL_MIN_DISK_GB=1 \
	sh /root/install.sh >/dev/null

echo "== upgrade"
docker exec "$name" timeout 300 env LEVELRAIL_BINARY_FILE=/root/levelrail sh /root/install.sh upgrade

echo "== uninstall"
in_ct 'sh /root/install.sh uninstall'
if in_ct 'systemctl is-active --quiet levelrail'; then echo "service still active"; exit 1; fi
in_ct 'test ! -e /usr/local/bin/levelrail && test ! -e /etc/systemd/system/levelrail.service'
in_ct 'test -f /var/lib/levelrail-data/levelrail.db' || { echo "data dir should survive uninstall"; exit 1; }
in_ct 'sh /root/install.sh uninstall --purge'
in_ct 'test ! -e /var/lib/levelrail-data' || { echo "--purge should delete the data dir"; exit 1; }

echo "== release verification (mirror, no cosign in the container)"
in_ct 'command -v cosign' >/dev/null 2>&1 && { echo "test expects no cosign in the container"; exit 1; }
in_ct 'goarch=amd64; [ "$(uname -m)" = x86_64 ] || goarch=arm64
	asset=levelrail-linux-$goarch
	mkdir -p /srv/rel/v9.9.9/good /srv/rel/v9.9.9/tampered
	for d in good tampered; do printf "#!/bin/sh\nexit 1\n" > /srv/rel/v9.9.9/$d/$asset; done
	(cd /srv/rel/v9.9.9/good && sha256sum "$asset" > checksums.txt)
	printf "%064d  %s\n" 0 "$asset" > /srv/rel/v9.9.9/tampered/checksums.txt
	printf "#!/bin/sh\nexit 0\n" > /usr/local/bin/levelrail && chmod 755 /usr/local/bin/levelrail
	nohup python3 -m http.server 8099 --bind 127.0.0.1 --directory /srv/rel >/dev/null 2>&1 &
	sleep 1'

mirror_upgrade() {
	dir="$1"
	shift
	docker exec "$name" env LEVELRAIL_VERSION=v9.9.9 LEVELRAIL_RELEASE_BASE_URL="http://127.0.0.1:8099/v9.9.9/$dir" \
		LEVELRAIL_INSECURE_MIRROR=1 LEVELRAIL_HEALTH_WAIT=1 "$@" sh /root/install.sh upgrade 2>&1 || true
}

out="$(mirror_upgrade tampered)"
echo "$out" | grep -q "checksum mismatch" || { echo "tampered checksums.txt was not rejected: $out"; exit 1; }

out="$(mirror_upgrade good APP_INSTALL_VERIFY=require)"
echo "$out" | grep -q "APP_INSTALL_VERIFY=require but cosign is not installed" ||
	{ echo "require mode without cosign did not fail closed: $out"; exit 1; }
echo "$out" | grep -q "Checksum verified" && { echo "require mode must fail before trusting the checksum"; exit 1; }

out="$(mirror_upgrade good)"
echo "$out" | grep -q "Checksum verified" || { echo "good checksum was not accepted in auto mode: $out"; exit 1; }
echo "$out" | grep -q "cosign not found" || { echo "auto mode should say the signature was not checked: $out"; exit 1; }

out="$(mirror_upgrade good APP_INSTALL_VERIFY=bogus)"
echo "$out" | grep -q "APP_INSTALL_VERIFY must be" || { echo "invalid APP_INSTALL_VERIFY accepted: $out"; exit 1; }
in_ct 'sh /root/install.sh uninstall --purge' >/dev/null

echo "install.sh passed on ${base_image}"
