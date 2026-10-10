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
in_ct 'grep -qx "Environment=APP_HTTP_ADDR=:8080" /etc/systemd/system/levelrail.service.d/10-install.conf && grep -qx "Environment=APP_INGRESS_HTTP_ADDR=:80" /etc/systemd/system/levelrail.service.d/10-install.conf' ||
	{ echo "free ports: drop-in should hold the default :8080/:80 addresses"; exit 1; }
perms="$(in_ct 'stat -c %a /var/lib/levelrail-data/setup-token')"
[ "$perms" = "600" ] || { echo "setup-token perms $perms, want 600"; exit 1; }
in_ct 'APP_DATA_DIR=/var/lib/levelrail-data /usr/local/bin/levelrail setup-token' | grep -qF -- "$(in_ct 'cat /var/lib/levelrail-data/setup-token')" ||
	{ echo "setup-token subcommand did not print the token file's token"; exit 1; }

echo "== uninstall before port-conflict tests"
in_ct 'sh /root/install.sh uninstall --purge' >/dev/null

echo "== dashboard port auto-picks the next free port when 8080 is taken"
in_ct 'nohup python3 -m http.server 8080 --bind 127.0.0.1 >/dev/null 2>&1 & echo $! > /tmp/blocker.pid'
out="$(docker exec "$name" timeout "$wait_secs" env \
	LEVELRAIL_BINARY_FILE=/root/levelrail LEVELRAIL_PUBLIC_IP=127.0.0.1 LEVELRAIL_MIN_DISK_GB=1 \
	sh /root/install.sh)"
echo "$out" | grep -q "taken, using 8081 instead" || { echo "did not auto-pick 8081: $out"; exit 1; }
in_ct 'curl -fsS --max-time 5 http://127.0.0.1:8081/healthz' >/dev/null ||
	{ echo "control plane did not come up on the auto-picked port 8081"; exit 1; }
# shellcheck disable=SC2016 # runs inside the container, expands there
in_ct 'kill "$(cat /tmp/blocker.pid)"' >/dev/null 2>&1 || true
in_ct 'sh /root/install.sh uninstall --purge' >/dev/null

echo "== ingress port conflict fails preflight, with a next-action message"
in_ct 'nohup python3 -m http.server 80 --bind 127.0.0.1 >/dev/null 2>&1 & echo $! > /tmp/blocker.pid'
rc=0
out="$(docker exec "$name" env \
	LEVELRAIL_BINARY_FILE=/root/levelrail LEVELRAIL_PUBLIC_IP=127.0.0.1 LEVELRAIL_MIN_DISK_GB=1 \
	sh /root/install.sh 2>&1)" || rc=$?
[ "$rc" -ne 0 ] || { echo "install should have failed with port 80 taken"; exit 1; }
echo "$out" | grep -q "LEVELRAIL_HTTP_PORT/LEVELRAIL_HTTPS_PORT" || { echo "fail message did not mention the override vars: $out"; exit 1; }
echo "$out" | grep -q "python3" || { echo "preflight did not name the process holding port 80: $out"; exit 1; }
echo "$out" | grep -q -- "--coexist" || { echo "preflight did not offer --coexist: $out"; exit 1; }
in_ct 'test ! -e /etc/systemd/system/levelrail.service.d/10-install.conf' || { echo "failed preflight must not write the drop-in"; exit 1; }

echo "== a Docker container publishing the port is named (fake docker on PATH)"
# shellcheck disable=SC2016 # runs inside the container, expands there
in_ct 'mkdir -p /fake && cat > /fake/docker <<"SH"
#!/bin/sh
case "$1" in
version) echo 27.0.1 ;;
ps) echo "container coolify-proxy (image traefik:v3.1)" ;;
esac
SH
chmod 755 /fake/docker'
rc=0
out="$(docker exec "$name" env PATH="/fake:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin" \
	LEVELRAIL_BINARY_FILE=/root/levelrail LEVELRAIL_PUBLIC_IP=127.0.0.1 LEVELRAIL_MIN_DISK_GB=1 \
	sh /root/install.sh 2>&1)" || rc=$?
[ "$rc" -ne 0 ] || { echo "install should have failed with port 80 taken"; exit 1; }
echo "$out" | grep -q "container coolify-proxy (image traefik:v3.1)" || { echo "docker holder not reported: $out"; exit 1; }

echo "== --yes switches to coexist mode, settings land in the drop-in"
out="$(docker exec "$name" timeout "$wait_secs" env \
	LEVELRAIL_BINARY_FILE=/root/levelrail LEVELRAIL_PUBLIC_IP=127.0.0.1 LEVELRAIL_MIN_DISK_GB=1 \
	sh /root/install.sh --yes --domain apps.example.test 2>&1)"
echo "$out" | grep -q "levelrail-cli proxy --domain apps.example.test --verify" || { echo "coexist summary missing the proxy command: $out"; exit 1; }
dropin=/etc/systemd/system/levelrail.service.d/10-install.conf
in_ct "grep -qx 'Environment=APP_HTTP_ADDR=127.0.0.1:8080' $dropin" || { echo "dashboard not on loopback in the drop-in"; exit 1; }
in_ct "grep -qx 'Environment=APP_INGRESS_HTTP_ADDR=:8088' $dropin" || { echo "ingress http not 8088 in the drop-in"; exit 1; }
in_ct "grep -qx 'Environment=APP_INGRESS_HTTPS_ADDR=:8443' $dropin" || { echo "ingress https not 8443 in the drop-in"; exit 1; }
in_ct '! grep -q "_ADDR=" /etc/systemd/system/levelrail.service' || { echo "main unit must not carry address settings"; exit 1; }
in_ct 'curl -fsS --max-time 5 http://127.0.0.1:8080/healthz' >/dev/null || { echo "control plane unhealthy in coexist mode"; exit 1; }
in_ct 'ss -ltn "sport = :8080"' | grep -q '127.0.0.1:8080' || { echo "dashboard is not bound to loopback only"; exit 1; }
in_ct 'ss -ltn "sport = :8088"' | grep -q ':8088' || { echo "nothing listens on ingress port 8088"; exit 1; }

echo "== upgrade keeps the drop-in and warns on a drifted main unit"
before="$(in_ct "md5sum $dropin")"
in_ct 'echo "# local edit" >> /etc/systemd/system/levelrail.service'
out="$(docker exec "$name" timeout 300 env LEVELRAIL_BINARY_FILE=/root/levelrail sh /root/install.sh upgrade 2>&1)"
[ "$before" = "$(in_ct "md5sum $dropin")" ] || { echo "upgrade rewrote the drop-in"; exit 1; }
echo "$out" | grep -q "differs from what this installer writes" || { echo "no drift warning: $out"; exit 1; }
in_ct 'curl -fsS --max-time 5 http://127.0.0.1:8080/healthz' >/dev/null || { echo "unhealthy after upgrade"; exit 1; }
in_ct 'ss -ltn "sport = :8088"' | grep -q ':8088' || { echo "upgrade reset the ingress port"; exit 1; }
# socket activation switch rewrites the main unit: ports must still survive
docker exec "$name" timeout 300 env LEVELRAIL_BINARY_FILE=/root/levelrail LEVELRAIL_SOCKET_ACTIVATION=0 sh /root/install.sh upgrade >/dev/null 2>&1
in_ct "grep -qx 'Environment=APP_INGRESS_HTTP_ADDR=:8088' $dropin" || { echo "unit rewrite lost the ingress port"; exit 1; }
# Self-bound ingress only opens the HTTPS port until an HTTP route exists.
in_ct 'ss -ltn "sport = :8443"' | grep -q ':8443' || {
	echo "ingress not on 8443 after the unit rewrite"
	in_ct 'ss -ltnp; systemctl cat levelrail; journalctl -u levelrail -n 30 --no-pager' || true
	exit 1
}

echo "== upgrade migrates a legacy unit's ports into the drop-in"
in_ct 'systemctl stop levelrail levelrail-http.socket levelrail-https.socket 2>/dev/null; rm -rf /etc/systemd/system/levelrail.service.d'
in_ct 'sed -i -e "/^Environment=APP_DATA_DIR/a Environment=APP_HTTP_ADDR=:8090" -e "/^Environment=APP_DATA_DIR/a Environment=APP_INGRESS_HTTP_ADDR=:8088" -e "/^Environment=APP_DATA_DIR/a Environment=APP_INGRESS_HTTPS_ADDR=:8443" /etc/systemd/system/levelrail.service && systemctl daemon-reload'
docker exec "$name" timeout 300 env LEVELRAIL_BINARY_FILE=/root/levelrail sh /root/install.sh upgrade >/dev/null 2>&1
in_ct "grep -qx 'Environment=APP_HTTP_ADDR=:8090' $dropin" || { echo "legacy dashboard port not migrated"; exit 1; }
in_ct 'curl -fsS --max-time 5 http://127.0.0.1:8090/healthz' >/dev/null || { echo "control plane not on the legacy dashboard port 8090"; exit 1; }
in_ct 'sh /root/install.sh uninstall --purge' >/dev/null
in_ct 'test ! -e /etc/systemd/system/levelrail.service.d/10-install.conf' || { echo "uninstall left the drop-in"; exit 1; }

echo "== explicit alternate ingress ports work around the same conflict"
docker exec "$name" timeout "$wait_secs" env \
	LEVELRAIL_BINARY_FILE=/root/levelrail LEVELRAIL_PUBLIC_IP=127.0.0.1 LEVELRAIL_MIN_DISK_GB=1 \
	LEVELRAIL_HTTP_PORT=8880 LEVELRAIL_HTTPS_PORT=8443 \
	sh /root/install.sh >/dev/null
in_ct 'curl -fsS --max-time 5 http://127.0.0.1:8080/healthz' >/dev/null ||
	{ echo "control plane did not come up with alternate ingress ports"; exit 1; }
# shellcheck disable=SC2016 # runs inside the container, expands there
in_ct 'kill "$(cat /tmp/blocker.pid)"' >/dev/null 2>&1 || true
in_ct 'sh /root/install.sh uninstall --purge' >/dev/null

echo "== reinstall for the remaining tests"
docker exec "$name" timeout "$wait_secs" env \
	LEVELRAIL_BINARY_FILE=/root/levelrail LEVELRAIL_PUBLIC_IP=127.0.0.1 LEVELRAIL_MIN_DISK_GB=1 \
	sh /root/install.sh >/dev/null

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
# shellcheck disable=SC2016 # runs inside the container, expands there
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
		LEVELRAIL_INSECURE_MIRROR=1 LEVELRAIL_HEALTH_WAIT=1 LEVELRAIL_NO_COSIGN=1 "$@" sh /root/install.sh upgrade 2>&1 || true
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

echo "== cosign is installed by default, verified against a pinned checksum"
# shellcheck disable=SC2016 # runs inside the container, expands there
in_ct 'goarch=amd64; [ "$(uname -m)" = x86_64 ] || goarch=arm64
	mkdir -p /srv/rel/cosign
	printf "#!/bin/sh\nexit 0\n" > /srv/rel/cosign/cosign-linux-$goarch
	sha256sum /srv/rel/cosign/cosign-linux-$goarch | cut -d" " -f1 > /srv/rel/cosign/sha
	echo http://127.0.0.1:8099/cosign/cosign-linux-$goarch > /srv/rel/cosign/url'
cosign_url="$(in_ct 'cat /srv/rel/cosign/url')"
cosign_sha="$(in_ct 'cat /srv/rel/cosign/sha')"
cosign_mirror() {
	dir="$1"
	shift
	mirror_upgrade "$dir" LEVELRAIL_NO_COSIGN=0 LEVELRAIL_COSIGN_URL="$cosign_url" LEVELRAIL_COSIGN_SHA256="$cosign_sha" "$@"
}

out="$(cosign_mirror good)"
echo "$out" | grep -q "Installed cosign" || { echo "cosign was not installed when absent: $out"; exit 1; }
echo "$out" | grep -q "Checksum verified" || { echo "install did not continue after cosign: $out"; exit 1; }
[ "$(in_ct 'stat -c %a /usr/local/bin/cosign')" = "755" ] || { echo "cosign not mode 755"; exit 1; }

out="$(cosign_mirror good)"
echo "$out" | grep -q "cosign already installed" || { echo "present cosign was not left alone: $out"; exit 1; }
echo "$out" | grep -q "Installing cosign" && { echo "cosign reinstalled although present"; exit 1; }
in_ct 'rm -f /usr/local/bin/cosign'

out="$(cosign_mirror good LEVELRAIL_NO_COSIGN=1)"
echo "$out" | grep -q "Installing cosign" && { echo "LEVELRAIL_NO_COSIGN=1 still installed cosign: $out"; exit 1; }
in_ct 'test ! -e /usr/local/bin/cosign' || { echo "opt-out left a cosign binary"; exit 1; }
out="$(docker exec "$name" env LEVELRAIL_VERSION=v9.9.9 LEVELRAIL_RELEASE_BASE_URL="http://127.0.0.1:8099/v9.9.9/good" \
	LEVELRAIL_INSECURE_MIRROR=1 LEVELRAIL_HEALTH_WAIT=1 LEVELRAIL_COSIGN_URL="$cosign_url" LEVELRAIL_COSIGN_SHA256="$cosign_sha" \
	sh /root/install.sh upgrade --no-cosign 2>&1 || true)"
in_ct 'test ! -e /usr/local/bin/cosign' || { echo "--no-cosign installed cosign: $out"; exit 1; }
echo "$out" | grep -q "Checksum verified" || { echo "--no-cosign broke the install: $out"; exit 1; }

out="$(mirror_upgrade good LEVELRAIL_NO_COSIGN=0 LEVELRAIL_COSIGN_URL=http://127.0.0.1:8099/cosign/missing LEVELRAIL_COSIGN_SHA256="$cosign_sha")"
echo "$out" | grep -q "could not download cosign" || { echo "failed cosign download not reported: $out"; exit 1; }
echo "$out" | grep -q "Checksum verified" || { echo "failed cosign download must degrade, not abort: $out"; exit 1; }
in_ct 'test ! -e /usr/local/bin/cosign' || { echo "failed download left a cosign binary"; exit 1; }

out="$(mirror_upgrade good LEVELRAIL_NO_COSIGN=0 LEVELRAIL_COSIGN_URL="$cosign_url" LEVELRAIL_COSIGN_SHA256=0000000000000000000000000000000000000000000000000000000000000000)"
echo "$out" | grep -q "cosign checksum mismatch" || { echo "wrong cosign checksum not rejected: $out"; exit 1; }
echo "$out" | grep -q "Checksum verified" || { echo "cosign mismatch must degrade, not abort: $out"; exit 1; }
in_ct 'test ! -e /usr/local/bin/cosign' || { echo "mismatched cosign was installed"; exit 1; }

echo "== install-cosign subcommand"
docker exec "$name" env LEVELRAIL_INSECURE_MIRROR=1 LEVELRAIL_COSIGN_URL="$cosign_url" LEVELRAIL_COSIGN_SHA256="$cosign_sha" \
	sh /root/install.sh install-cosign >/dev/null
in_ct 'test -x /usr/local/bin/cosign' || { echo "install-cosign did not install cosign"; exit 1; }
in_ct 'rm -f /usr/local/bin/cosign'
rc=0
docker exec "$name" env LEVELRAIL_INSECURE_MIRROR=1 LEVELRAIL_COSIGN_URL=http://127.0.0.1:8099/cosign/missing LEVELRAIL_COSIGN_SHA256="$cosign_sha" \
	sh /root/install.sh install-cosign >/dev/null 2>&1 || rc=$?
[ "$rc" -ne 0 ] || { echo "install-cosign must fail loudly when asked explicitly"; exit 1; }
in_ct 'sh /root/install.sh uninstall --purge' >/dev/null

echo "install.sh passed on ${base_image}"
