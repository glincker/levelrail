#!/bin/sh
# curl -fsSL https://raw.githubusercontent.com/glincker/levelrail/main/install.sh | sudo sh
#
# Installs the levelrail control plane binary, a systemd unit, and Docker
# (if missing) on a single Linux host. Safe to re-run: it overwrites the
# binary and unit file, then restarts the service, so re-running this
# script is also how you upgrade.
#
# Env overrides:
#   LEVELRAIL_VERSION      release tag to install, e.g. v0.3.0 (default: latest)
#   LEVELRAIL_INSTALL_DIR  where the binary goes (default: /usr/local/bin)
#   LEVELRAIL_DATA_DIR     control plane data dir (default: /var/lib/levelrail-data)

set -eu

REPO="glincker/levelrail"
BINARY_NAME="levelrail"
INSTALL_DIR="${LEVELRAIL_INSTALL_DIR:-/usr/local/bin}"
DATA_DIR="${LEVELRAIL_DATA_DIR:-/var/lib/levelrail-data}"
UNIT_PATH="/etc/systemd/system/levelrail.service"

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
	curl -fsSL --proto '=https' --tlsv1.2 https://get.docker.com | sh
fi
systemctl enable --now docker

VERSION="${LEVELRAIL_VERSION:-}"
if [ -z "$VERSION" ]; then
	VERSION="$(curl -fsSL --proto '=https' --tlsv1.2 "https://api.github.com/repos/${REPO}/releases/latest" 2>/dev/null |
		grep -m1 '"tag_name"' | sed -E 's/.*"tag_name": *"([^"]+)".*/\1/')"
	[ -n "$VERSION" ] || fatal "could not resolve the latest release (none published yet?). Set LEVELRAIL_VERSION=vX.Y.Z to install a specific version."
fi

asset="levelrail-linux-${goarch}"
base_url="https://github.com/${REPO}/releases/download/${VERSION}"

tmp_dir="$(mktemp -d)"
trap 'rm -rf "$tmp_dir"' EXIT

log "Downloading ${asset} ${VERSION}..."
curl -fsSL --proto '=https' --tlsv1.2 -o "$tmp_dir/$asset" "${base_url}/${asset}" || fatal "download failed: ${base_url}/${asset}"

if curl -fsSL --proto '=https' --tlsv1.2 -o "$tmp_dir/checksums.txt" "${base_url}/checksums.txt" 2>/dev/null; then
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

systemctl daemon-reload
systemctl enable levelrail
systemctl restart levelrail

ip_addr="$(hostname -I 2>/dev/null | awk '{print $1}')"
[ -n "$ip_addr" ] || ip_addr="<server-ip>"

cat <<EOF

Levelrail ${VERSION} installed and running.

  Dashboard:      http://${ip_addr}:8080
  Service status: systemctl status levelrail
  Logs:           journalctl -u levelrail -f
  Data directory: ${DATA_DIR}

Create the initial admin account:
  sudo APP_DATA_DIR=${DATA_DIR} ${INSTALL_DIR}/${BINARY_NAME} recover-admin --username admin
EOF
