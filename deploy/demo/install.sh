#!/usr/bin/env bash
# Idempotent provisioning of the read-only demo on a fresh Debian or Ubuntu box.
# Run as root from the unpacked levelrail-demo bundle. Re-running only converges
# files, the user and the units, and never touches existing demo data.
set -euo pipefail

[ "$(id -u)" -eq 0 ] || { echo "run as root" >&2; exit 1; }
here="$(cd "$(dirname "$0")" && pwd)"
[ -f "$here/levelrail" ] || { echo "levelrail binary not found next to install.sh" >&2; exit 1; }

export DEBIAN_FRONTEND=noninteractive
apt-get update -qq
apt-get install -y -qq sqlite3 ca-certificates >/dev/null

id -u levelrail-demo >/dev/null 2>&1 ||
	useradd --system --home-dir /var/lib/levelrail-demo --shell /usr/sbin/nologin levelrail-demo

install -d -m 0755 /opt/levelrail-demo/bin /usr/local/lib/levelrail-demo
install -d -m 0750 -o levelrail-demo -g levelrail-demo /var/lib/levelrail-demo
install -d -m 0750 -o root -g levelrail-demo /etc/levelrail-demo

install -m 0755 "$here/levelrail" /opt/levelrail-demo/bin/levelrail
install -m 0755 "$here/reseed.sh" /usr/local/lib/levelrail-demo/reseed.sh
[ -f /etc/levelrail-demo/env ] ||
	install -m 0640 -o root -g levelrail-demo "$here/env.example" /etc/levelrail-demo/env

for u in levelrail-demo.service levelrail-demo-reseed.service levelrail-demo-reseed.timer \
	levelrail-demo-tripwire.service levelrail-demo-tripwire.timer; do
	install -m 0644 "$here/$u" "/etc/systemd/system/$u"
done
systemctl daemon-reload

if [ ! -d /var/lib/levelrail-demo/data ]; then
	systemctl start levelrail-demo-reseed.service
fi
systemctl enable --now levelrail-demo.service \
	levelrail-demo-reseed.timer levelrail-demo-tripwire.timer

echo "demo listening on 127.0.0.1:8080; point your tunnel at it (see cloudflared-config.yml.example)"
