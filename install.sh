#!/bin/sh
# curl -fsSL https://raw.githubusercontent.com/glincker/levelrail/main/install.sh | sudo sh
#
# Installs the levelrail control plane binary, a systemd unit, and Docker
# (if missing) on a single Linux host. Safe to re-run: it overwrites the
# binary and unit file, then restarts the service, so re-running this
# script is also how you upgrade.
#
# Env overrides:
#   LEVELRAIL_VERSION       release tag to install, e.g. v0.3.0 (default: latest)
#   LEVELRAIL_INSTALL_DIR   where the binary goes (default: /usr/local/bin)
#   LEVELRAIL_DATA_DIR      control plane data dir (default: /var/lib/levelrail-data)
#   LEVELRAIL_CONFIGURE_UFW set to 1 to have this script configure ufw
#                           (allow SSH, then 80/443, then enable it if
#                           not already active). Off by default: this
#                           script never touches your firewall unless
#                           you explicitly ask it to. See the "Firewall"
#                           section below for exactly what it does and
#                           in what order.

set -eu

REPO="glincker/levelrail"
BINARY_NAME="levelrail"
INSTALL_DIR="${LEVELRAIL_INSTALL_DIR:-/usr/local/bin}"
DATA_DIR="${LEVELRAIL_DATA_DIR:-/var/lib/levelrail-data}"
UNIT_PATH="/etc/systemd/system/levelrail.service"
HTTPS_ONLY="=https"
HEALTH_CHECK_MAX_WAIT=60

log() { printf '%s\n' "$*"; }
fatal() { printf 'error: %s\n' "$*" >&2; exit 1; }

[ "$(id -u)" -eq 0 ] || fatal "must run as root, e.g.: curl -fsSL <url> | sudo sh"

os="$(uname -s)"
[ "$os" = "Linux" ] || fatal "levelrail only supports Linux, detected: $os"

arch="$(uname -m)"
case "$arch" in
	x86_64 | amd64) goarch="amd64" ;;
	aarch64 | arm64) goarch="arm64" ;;
	*) fatal "unsupported architecture: $arch (levelrail ships linux/amd64 and linux/arm64)" ;;
esac

command -v curl >/dev/null 2>&1 || fatal "curl is required"
command -v systemctl >/dev/null 2>&1 || fatal "systemd is required"

if command -v docker >/dev/null 2>&1; then
	log "Docker already installed, skipping."
else
	log "Docker not found, installing via get.docker.com..."
	curl -fsSL --proto "$HTTPS_ONLY" --tlsv1.2 https://get.docker.com | sh
fi
systemctl enable --now docker

VERSION="${LEVELRAIL_VERSION:-}"
if [ -z "$VERSION" ]; then
	VERSION="$(curl -fsSL --proto "$HTTPS_ONLY" --tlsv1.2 "https://api.github.com/repos/${REPO}/releases/latest" 2>/dev/null |
		grep -m1 '"tag_name"' | sed -E 's/.*"tag_name": *"([^"]+)".*/\1/')"
	[ -n "$VERSION" ] || fatal "could not resolve the latest release (none published yet?). Set LEVELRAIL_VERSION=vX.Y.Z to install a specific version."
fi

asset="levelrail-linux-${goarch}"
base_url="https://github.com/${REPO}/releases/download/${VERSION}"

tmp_dir="$(mktemp -d)"
trap 'rm -rf "$tmp_dir"' EXIT

log "Downloading ${asset} ${VERSION}..."
curl -fsSL --proto "$HTTPS_ONLY" --tlsv1.2 -o "$tmp_dir/$asset" "${base_url}/${asset}" || fatal "download failed: ${base_url}/${asset}"

if curl -fsSL --proto "$HTTPS_ONLY" --tlsv1.2 -o "$tmp_dir/checksums.txt" "${base_url}/checksums.txt" 2>/dev/null; then
	expected="$(grep " ${asset}\$" "$tmp_dir/checksums.txt" | awk '{print $1}')"
	if [ -n "$expected" ]; then
		actual="$(sha256sum "$tmp_dir/$asset" | awk '{print $1}')"
		[ "$expected" = "$actual" ] || fatal "checksum mismatch for ${asset}: expected ${expected}, got ${actual}"
		log "Checksum verified."
	else
		log "warning: ${asset} not listed in checksums.txt, skipping verification"
	fi
else
	log "warning: no checksums.txt published for ${VERSION}, skipping verification"
fi

install -m 0755 "$tmp_dir/$asset" "${INSTALL_DIR}/${BINARY_NAME}"

mkdir -p "$DATA_DIR"
if [ ! -f "$DATA_DIR/brand.yaml" ]; then
	cat >"$DATA_DIR/brand.yaml" <<-EOF
		name: Levelrail
		short_name: Levelrail
		binary_name: levelrail
		domain: glinr.com/levelrail
		support_url: https://github.com/${REPO}/issues
		docs_url: https://glinr.com/levelrail/docs
		primary_color: ""
		logo_svg: ""
	EOF
fi

cat >"$UNIT_PATH" <<EOF
[Unit]
Description=Levelrail control plane
After=network-online.target docker.service
Requires=docker.service
Wants=network-online.target

[Service]
ExecStart=${INSTALL_DIR}/${BINARY_NAME}
WorkingDirectory=${DATA_DIR}
Environment=APP_DATA_DIR=${DATA_DIR}
Restart=on-failure
RestartSec=5

[Install]
WantedBy=multi-user.target
EOF

# Firewall (opt-in, LEVELRAIL_CONFIGURE_UFW=1 only): allow SSH before
# touching anything else, and only enable ufw if it wasn't already
# active. Getting this order wrong (enabling before SSH is allowed) is
# the classic way an install script locks an operator out of their own
# server; the ordering here matches Dokku's own DigitalOcean image
# provisioning script (allow ssh, allow web ports, enable last).
if [ "${LEVELRAIL_CONFIGURE_UFW:-0}" = "1" ]; then
	if command -v ufw >/dev/null 2>&1; then
		log "Configuring ufw (LEVELRAIL_CONFIGURE_UFW=1)..."
		was_active=0
		ufw status 2>/dev/null | grep -q "^Status: active" && was_active=1
		ufw allow OpenSSH >/dev/null 2>&1 || ufw allow 22/tcp >/dev/null 2>&1
		ufw allow 80/tcp >/dev/null 2>&1
		ufw allow 443/tcp >/dev/null 2>&1
		if [ "$was_active" -eq 1 ]; then
			log "ufw was already active, added rules without re-enabling."
		else
			ufw --force enable >/dev/null 2>&1
			log "ufw enabled: SSH, 80/tcp, and 443/tcp are allowed, everything else denied by default."
		fi
	else
		log "LEVELRAIL_CONFIGURE_UFW=1 set, but ufw is not installed, skipping."
	fi
fi

systemctl daemon-reload
systemctl enable levelrail
systemctl restart levelrail

# Bounded health check: confirm the control plane actually came up
# before declaring success, rather than trusting systemctl restart's
# own immediate (and uninformative) exit code. Never an infinite loop:
# HEALTH_CHECK_MAX_WAIT caps it, and a timeout is a real, actionable
# failure (exit 1 with the exact command to inspect why), the same
# bounded-wait-then-fail-loudly shape Coolify's own install script
# uses for its own post-install health verification.
log "Waiting for the control plane to come up..."
waited=0
healthy=0
while [ "$waited" -lt "$HEALTH_CHECK_MAX_WAIT" ]; do
	if curl -fsS -o /dev/null "http://127.0.0.1:8080/api/v1/brand" 2>/dev/null; then
		healthy=1
		break
	fi
	sleep 2
	waited=$((waited + 2))
done

if [ "$healthy" -ne 1 ]; then
	fatal "control plane did not become healthy within ${HEALTH_CHECK_MAX_WAIT}s. Check: systemctl status levelrail && journalctl -u levelrail -n 100 --no-pager"
fi
log "Control plane is healthy."

ip_addr="$(hostname -I 2>/dev/null | awk '{print $1}')"
[ -n "$ip_addr" ] || ip_addr="<server-ip>"

cat <<EOF

Levelrail ${VERSION} installed and running.

  Dashboard:      http://${ip_addr}:8080
  Service status: systemctl status levelrail
  Logs:           journalctl -u levelrail -f
  Data directory: ${DATA_DIR}

Create the initial admin account by visiting the dashboard above and
using the "Set up admin account" tab (this only works once, for the very
first account on the instance). To automate this instead of doing it by
hand, set APP_ADMIN_USERNAME and APP_ADMIN_PASSWORD in the systemd unit's
[Service] section before the first start.

Locked out of an existing admin account later? That is what recover-admin
is for, it resets a password (or creates a fallback account if the
username you give it does not exist yet), it does not set up the first
account on a fresh install:
  sudo APP_DATA_DIR=${DATA_DIR} ${INSTALL_DIR}/${BINARY_NAME} recover-admin --username admin

Re-run this script any time to repair or upgrade: it overwrites the
binary and unit file, re-checks the firewall (if LEVELRAIL_CONFIGURE_UFW=1),
and re-verifies the control plane comes back up healthy.
EOF
