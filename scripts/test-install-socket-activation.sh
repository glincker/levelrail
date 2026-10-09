#!/bin/sh
# Verifies the default socket-activated install inside a systemd container:
#   scripts/test-install-socket-activation.sh <base-image> <path-to-linux-binary>
# systemd must own 80/443, and restarting the control plane must not refuse a
# single TCP connection to them.
set -eu

base_image="$1"
binary="$2"
repo_root="$(cd "$(dirname "$0")/.." && pwd)"
tag="install-sd-test:$(printf '%s' "$base_image" | tr ':/' '--')"
name="install-sd-test-$$"
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
env_args="LEVELRAIL_BINARY_FILE=/root/levelrail LEVELRAIL_PUBLIC_IP=127.0.0.1 LEVELRAIL_MIN_DISK_GB=1 LEVELRAIL_SKIP_REACHABILITY=1"

echo "== default install uses socket activation"
docker exec "$name" timeout "$wait_secs" env $env_args sh /root/install.sh >/dev/null
in_ct 'systemctl is-active --quiet levelrail-http.socket && systemctl is-active --quiet levelrail-https.socket' ||
	{ echo "socket units are not active"; exit 1; }
in_ct 'curl -fsS --max-time 5 http://127.0.0.1:8080/healthz' >/dev/null || { echo "control plane unhealthy"; exit 1; }

echo "== systemd, not the control plane, holds 80 and 443"
in_ct 'ss -ltnp "sport = :443"' | grep -q 'systemd' || { echo "443 is not held by systemd"; in_ct 'ss -ltnp "sport = :443"'; exit 1; }
in_ct 'ss -ltnp "sport = :80"' | grep -q 'systemd' || { echo "80 is not held by systemd"; exit 1; }
in_ct 'journalctl -u levelrail --no-pager | grep -q "systemd socket activation"' || { echo "control plane did not report socket activation"; exit 1; }

echo "== http redirect is served from the inherited port 80"
code="$(in_ct 'curl -s -o /dev/null -w "%{http_code}" --max-time 5 -H "Host: example.test" http://127.0.0.1/')"
[ "$code" = "308" ] || [ "$code" = "404" ] || [ "$code" = "200" ] || { echo "port 80 answered $code"; exit 1; }

echo "== restart refuses no connection"
in_ct 'cat > /tmp/probe.py <<PY
import socket, sys, time
fails = ok = 0
end = time.time() + float(sys.argv[1])
while time.time() < end:
    for port in (80, 443):
        try:
            socket.create_connection(("127.0.0.1", port), timeout=2).close()
            ok += 1
        except OSError:
            fails += 1
    time.sleep(0.02)
print("ok=%d fails=%d" % (ok, fails))
sys.exit(1 if fails else 0)
PY'
in_ct 'python3 /tmp/probe.py 20 > /tmp/probe.out 2>&1 & echo $! > /tmp/probe.pid'
sleep 3
in_ct 'systemctl restart levelrail'
in_ct 'systemctl restart levelrail'
rc=0
in_ct 'wait "$(cat /tmp/probe.pid)" 2>/dev/null; sleep 18' || true
in_ct 'cat /tmp/probe.out'
in_ct 'grep -q "fails=0" /tmp/probe.out' || { echo "connections were refused during a restart"; exit 1; }

echo "== upgrade back to a self-bound ingress"
docker exec "$name" timeout 300 env $env_args LEVELRAIL_SOCKET_ACTIVATION=0 sh /root/install.sh upgrade >/dev/null
if in_ct 'systemctl is-active --quiet levelrail-https.socket'; then echo "socket unit still active"; exit 1; fi
in_ct 'ss -ltnp "sport = :443"' | grep -q levelrail || { echo "control plane does not bind 443 itself after switching back"; exit 1; }

echo "== uninstall removes the socket units"
in_ct 'sh /root/install.sh uninstall --purge' >/dev/null
in_ct 'test ! -e /etc/systemd/system/levelrail-https.socket' || { echo "socket unit left behind"; exit 1; }
echo "socket activation install test passed (rc=$rc)"
