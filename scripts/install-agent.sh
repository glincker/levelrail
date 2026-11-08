#!/bin/sh
# Joins this server to a control plane as a node.
#   install-agent.sh --server HOST:9443 --ca-fingerprint FP --token-stdin [--name N] [--mesh]
#                    [--version vX.Y.Z] [--wait SECONDS] [--dry-run]
#   install-agent.sh --uninstall [--dry-run]
# Mint the token and see the exact command with: levelrail-cli nodes add --name N
# The token is read from stdin or a file so it never shows up in the process list.
set -eu

REPO="glincker/levelrail"
AGENT="levelrail-agent"
BIN_PATH="${LEVELRAIL_AGENT_BIN:-/usr/local/bin/${AGENT}}"
CONF_DIR="${LEVELRAIL_AGENT_CONF_DIR:-/etc/${AGENT}}"
ENV_FILE="${CONF_DIR}/agent.env"
UNIT_PATH="${LEVELRAIL_AGENT_UNIT:-/etc/systemd/system/${AGENT}.service}"
STATE_DIR="${LEVELRAIL_AGENT_STATE_DIR:-/var/lib/${AGENT}}"
HTTPS_ONLY="=https"
PROTO="$HTTPS_ONLY"
VERIFY_MODE="${APP_INSTALL_VERIFY:-auto}"
COSIGN_IDENTITY_REGEXP="https://github.com/${REPO}/\\.github/workflows/release\\.yml@.*"
COSIGN_OIDC_ISSUER="https://token.actions.githubusercontent.com"

SERVER=""
FINGERPRINT=""
NODE_NAME=""
MESH=0
VERSION="${LEVELRAIL_VERSION:-}"
WAIT="${LEVELRAIL_AGENT_WAIT:-60}"
TOKEN=""
TOKEN_FROM=""
DRY=0
UNINSTALL=0
PRINT_UNIT=0
GOARCH=""

log() { printf '%s\n' "$*"; }
warn() { printf 'warning: %s\n' "$*" >&2; }
fatal() {
	printf 'error: %s\n' "$*" >&2
	exit 1
}
# act prints the action in dry-run mode and runs it otherwise.
act() {
	if [ "$DRY" -eq 1 ]; then
		log "[dry-run] $*"
	else
		"$@"
	fi
}

usage() {
	cat <<USAGE
Usage: install-agent.sh --server HOST:PORT --token-stdin|--token-file FILE [options]
       install-agent.sh --uninstall

  --server ADDR          the control plane agent address, host:9443 (an https:// prefix is accepted)
  --ca-fingerprint FP    control plane CA fingerprint from the join command (pins the first connection)
  --token-stdin          read the single-use join token from stdin
  --token-file FILE      read the join token from FILE
  --token TOKEN          pass the token as an argument (visible in the process list, avoid)
  --name NAME            node name (lowercase letters, digits, dashes)
  --mesh                 join the WireGuard mesh (APP_MESH_ENABLED=1, NET_ADMIN, /dev/net/tun)
  --version vX.Y.Z       release to install (default: newest published)
  --wait SECONDS         how long to wait for the agent to enroll (default 60)
  --dry-run              print every action and change nothing
  --uninstall            remove the unit, binary and env file; Docker data is never touched
USAGE
}

need_value() { [ "$1" -ge 2 ] || fatal "$2 needs a value"; }

while [ $# -gt 0 ]; do
	case "$1" in
	--server) need_value $# "$1"; SERVER="$2"; shift ;;
	--ca-fingerprint) need_value $# "$1"; FINGERPRINT="$2"; shift ;;
	--name) need_value $# "$1"; NODE_NAME="$2"; shift ;;
	--version) need_value $# "$1"; VERSION="$2"; shift ;;
	--wait) need_value $# "$1"; WAIT="$2"; shift ;;
	--token-file) need_value $# "$1"; TOKEN_FROM="file:$2"; shift ;;
	--token-stdin) TOKEN_FROM="stdin" ;;
	--token) need_value $# "$1"; TOKEN="$2"; TOKEN_FROM="arg"; shift ;;
	--mesh) MESH=1 ;;
	--dry-run) DRY=1 ;;
	--uninstall) UNINSTALL=1 ;;
	--print-unit) PRINT_UNIT=1 ;;
	-h | --help) usage; exit 0 ;;
	*) usage >&2; fatal "unknown option: $1" ;;
	esac
	shift
done

case "$WAIT" in
'' | *[!0-9]*) fatal "--wait must be a number of seconds" ;;
esac
case "$NODE_NAME" in
*[!a-z0-9-]*) fatal "--name must be lowercase letters, digits and dashes" ;;
esac

unit_text() {
	cat <<UNIT
[Unit]
Description=Levelrail node agent
Documentation=https://docs.levelrail.com/multi-node-quickstart
After=network-online.target docker.service
Wants=network-online.target
Requires=docker.service

[Service]
# Root: the agent talks to /var/run/docker.sock and owns the host side of the mesh.
User=root
EnvironmentFile=${ENV_FILE}
Environment=APP_AGENT_IDENTITY_FILE=${STATE_DIR}/identity.json
# Creates the state directory (0700) for the node identity and certificates.
StateDirectory=${AGENT}
StateDirectoryMode=0700
ExecStart=${BIN_PATH}
# The agent keeps containers running while it is down, so always come back.
Restart=always
RestartSec=5
# Stops the agent or its children from gaining privileges through setuid binaries.
NoNewPrivileges=true
# The agent never reads home directories.
ProtectHome=true
# Blocks setuid/setgid file creation. Docker is unaffected: the daemon creates containers, not the agent.
RestrictSUIDSGID=true
# PrivateTmp is left off on purpose: bind mounts and build contexts staged under /tmp must stay visible to the Docker daemon.
UNIT
	if [ "$MESH" -eq 1 ]; then
		cat <<UNIT
# Mesh: lets the agent create and configure the WireGuard interface.
AmbientCapabilities=CAP_NET_ADMIN
# Mesh: the userspace WireGuard device is a TUN device.
DeviceAllow=/dev/net/tun rw
UNIT
	fi
	cat <<UNIT

[Install]
WantedBy=multi-user.target
UNIT
}

if [ "$PRINT_UNIT" -eq 1 ]; then
	unit_text
	exit 0
fi

preflight_host() {
	[ "$DRY" -eq 0 ] || return 0
	[ "$(id -u)" -eq 0 ] || fatal "run as root. Fix: re-run with sudo."
	[ "$(uname -s)" = "Linux" ] || fatal "the agent runs on Linux only."
	command -v systemctl >/dev/null 2>&1 || fatal "systemd is required (systemctl not found). Fix: use a systemd-based distro, or run the agent container from the join command instead."
	[ -d /run/systemd/system ] || fatal "systemd is installed but not running as init. Fix: boot the host with systemd, or use the container join command."
	command -v curl >/dev/null 2>&1 || fatal "curl is required. Fix: install curl with your package manager and retry."
}

detect_arch() {
	case "$(uname -m)" in
	x86_64 | amd64) GOARCH="amd64" ;;
	aarch64 | arm64) GOARCH="arm64" ;;
	*) fatal "unsupported CPU $(uname -m): the agent ships for amd64 and arm64." ;;
	esac
}

# Same method as the control plane installer: the official convenience script.
ensure_docker() {
	if ! command -v docker >/dev/null 2>&1; then
		log "Installing Docker via get.docker.com..."
		if [ "$DRY" -eq 1 ]; then
			log "[dry-run] curl -fsSL https://get.docker.com | sh"
		else
			curl -fsSL --proto "$HTTPS_ONLY" --tlsv1.2 --connect-timeout 10 --max-time 120 https://get.docker.com | sh ||
				fatal "Docker install failed. Fix: install Docker manually (https://docs.docker.com/engine/install/) and re-run."
		fi
	fi
	act systemctl enable --now docker
}

read_token() {
	case "$TOKEN_FROM" in
	stdin) TOKEN="$(cat)" ;;
	file:*)
		f="${TOKEN_FROM#file:}"
		[ -r "$f" ] || fatal "cannot read the token file: $f"
		TOKEN="$(cat "$f")"
		;;
	arg) warn "--token puts the token in the process list; prefer --token-stdin or --token-file" ;;
	esac
	TOKEN="$(printf '%s' "$TOKEN" | tr -d ' \t\r\n')"
	case "$TOKEN" in
	*[!A-Za-z0-9._~+/=-]*) fatal "the join token has unexpected characters. Fix: mint a fresh one with: levelrail-cli nodes add" ;;
	esac
}

normalise_server() {
	SERVER="${SERVER#https://}"
	SERVER="${SERVER%/}"
	case "$SERVER" in
	*[!A-Za-z0-9.:_[\]-]*) fatal "--server must be host:port (got an unexpected character)" ;;
	esac
	case "$SERVER" in
	*:*) ;;
	*) SERVER="${SERVER}:9443" ;;
	esac
}

resolve_version() {
	[ -z "$VERSION" ] || return 0
	if [ -n "${LEVELRAIL_RELEASE_BASE_URL:-}" ]; then
		VERSION="$(curl -fsSL --proto "$PROTO" --tlsv1.2 --connect-timeout 10 --max-time 30 "${LEVELRAIL_RELEASE_BASE_URL%/}/latest" 2>/dev/null | head -n 1 | tr -d ' \t\r' || true)"
	else
		VERSION="$(curl -fsSL --proto "$HTTPS_ONLY" --tlsv1.2 --connect-timeout 10 --max-time 30 -H "Accept: application/vnd.github+json" \
			"https://api.github.com/repos/${REPO}/releases?per_page=1" 2>/dev/null | grep -m1 '"tag_name"' | sed 's/.*"tag_name"[^"]*"\([^"]*\)".*/\1/' || true)"
	fi
	[ -n "$VERSION" ] || fatal "could not work out the newest release. Fix: pass --version vX.Y.Z."
	case "$VERSION" in
	*[!A-Za-z0-9._+-]*) fatal "the release lookup returned something that is not a version tag." ;;
	esac
}

verify_signature() {
	[ "$VERIFY_MODE" != "off" ] || return 0
	if ! command -v cosign >/dev/null 2>&1; then
		[ "$VERIFY_MODE" != "require" ] || fatal "APP_INSTALL_VERIFY=require but cosign is not installed. Fix: install cosign or unset APP_INSTALL_VERIFY."
		log "cosign not found: verifying the SHA-256 checksum only."
		return 0
	fi
	bundle="$1.sigstore.json"
	if ! curl -fsSL --proto "$PROTO" --tlsv1.2 --connect-timeout 10 --max-time 60 -o "$bundle" "$2/checksums.txt.sigstore.json" 2>/dev/null; then
		[ "$VERIFY_MODE" != "require" ] || fatal "APP_INSTALL_VERIFY=require but ${VERSION} publishes no signature."
		warn "${VERSION} publishes no signature for checksums.txt, verifying the checksum only"
		return 0
	fi
	cosign verify-blob --bundle "$bundle" --certificate-identity-regexp "$COSIGN_IDENTITY_REGEXP" \
		--certificate-oidc-issuer "$COSIGN_OIDC_ISSUER" "$1" >/dev/null 2>&1 ||
		fatal "cosign signature check of checksums.txt failed for ${VERSION}, refusing to install."
	log "Signature verified (cosign, keyless)."
}

# Mirrors the control plane installer: checksum always, cosign when present, never silently skipped.
fetch_binary() {
	dest="$1"
	if [ -n "${LEVELRAIL_BINARY_FILE:-}" ]; then
		[ -f "$LEVELRAIL_BINARY_FILE" ] || fatal "LEVELRAIL_BINARY_FILE not found: $LEVELRAIL_BINARY_FILE"
		warn "a local binary is not checksum or signature verified"
		cp "$LEVELRAIL_BINARY_FILE" "$dest"
		return
	fi
	base_url="${LEVELRAIL_RELEASE_BASE_URL:-}"
	case "$base_url" in
	'' | https://*) ;;
	http://*)
		[ "${LEVELRAIL_INSECURE_MIRROR:-}" = "1" ] || fatal "LEVELRAIL_RELEASE_BASE_URL must be https (LEVELRAIL_INSECURE_MIRROR=1 allows http for tests)"
		PROTO="=http"
		;;
	*) fatal "LEVELRAIL_RELEASE_BASE_URL must start with https://" ;;
	esac
	resolve_version
	asset="${AGENT}-linux-${GOARCH}"
	base_url="${base_url:-https://github.com/${REPO}/releases/download/${VERSION}}"
	sums="${dest}.checksums"
	curl -fsSL --proto "$PROTO" --tlsv1.2 --connect-timeout 10 --max-time 60 -o "$sums" "${base_url}/checksums.txt" 2>/dev/null ||
		fatal "no checksums.txt for ${VERSION}, refusing to install an unverified binary. Fix: pick another release with --version."
	verify_signature "$sums" "$base_url"
	expected="$(grep " ${asset}\$" "$sums" | awk '{ print $1 }')"
	[ -n "$expected" ] || fatal "${asset} is not listed in checksums.txt for ${VERSION}. Fix: pick another release with --version."
	log "Downloading ${asset} ${VERSION}..."
	curl -fsSL --proto "$PROTO" --tlsv1.2 --connect-timeout 10 --max-time 300 -o "$dest" "${base_url}/${asset}" ||
		fatal "download failed: ${base_url}/${asset}. Fix: check outbound HTTPS to github.com or pick another release."
	actual="$(sha256sum "$dest" | awk '{ print $1 }')"
	[ "$expected" = "$actual" ] || fatal "checksum mismatch for ${asset}: expected ${expected}, got ${actual}"
	log "Checksum verified."
}

write_env() {
	umask 077
	if [ "$DRY" -eq 1 ]; then
		log "[dry-run] write ${ENV_FILE} (mode 0600) with APP_CONTROL_PLANE_ADDR=${SERVER}, APP_CA_FINGERPRINT and APP_NODE_NAME when given, APP_JOIN_TOKEN=<hidden> on a first install, APP_MESH_ENABLED=1 with --mesh"
		return
	fi
	mkdir -p "$CONF_DIR"
	tmp="${ENV_FILE}.new"
	{
		printf 'APP_CONTROL_PLANE_ADDR=%s\n' "$SERVER"
		[ -z "$FINGERPRINT" ] || printf 'APP_CA_FINGERPRINT=%s\n' "$FINGERPRINT"
		[ -z "$NODE_NAME" ] || printf 'APP_NODE_NAME=%s\n' "$NODE_NAME"
		[ -z "$TOKEN" ] || printf 'APP_JOIN_TOKEN=%s\n' "$TOKEN"
		[ "$MESH" -eq 0 ] || printf 'APP_MESH_ENABLED=1\n'
	} >"$tmp"
	chmod 0600 "$tmp"
	mv "$tmp" "$ENV_FILE"
}

# The token is single use, so drop it from disk once the identity exists.
scrub_token() {
	[ -f "$ENV_FILE" ] || return 0
	grep -q '^APP_JOIN_TOKEN=' "$ENV_FILE" || return 0
	tmp="${ENV_FILE}.new"
	umask 077
	grep -v '^APP_JOIN_TOKEN=' "$ENV_FILE" >"$tmp" || true
	chmod 0600 "$tmp"
	mv "$tmp" "$ENV_FILE"
}

wait_enrolled() {
	if [ "$DRY" -eq 1 ]; then
		log "[dry-run] wait up to ${WAIT}s for ${STATE_DIR}/identity.json and an active ${AGENT} service"
		return 0
	fi
	waited=0
	while [ "$waited" -lt "$WAIT" ]; do
		if [ -f "${STATE_DIR}/identity.json" ] && systemctl is-active --quiet "$AGENT"; then
			return 0
		fi
		sleep 2
		waited=$((waited + 2))
	done
	return 1
}

do_uninstall() {
	log "Removing the ${AGENT} service, binary and env file. Docker data and other software stay untouched."
	if [ "$DRY" -eq 0 ]; then
		[ "$(id -u)" -eq 0 ] || fatal "run as root. Fix: re-run with sudo."
	fi
	act systemctl disable --now "$AGENT" || true
	act rm -f "$UNIT_PATH" "$BIN_PATH" "$ENV_FILE"
	act rmdir "$CONF_DIR" || true
	act systemctl daemon-reload || true
	log "The node identity in ${STATE_DIR} was kept so a reinstall rejoins as the same node. Delete the node in the control plane, then remove that directory to forget it."
}

do_install() {
	[ -n "$SERVER" ] || { usage >&2; fatal "--server is required"; }
	normalise_server
	preflight_host
	detect_arch
	if [ -f "${STATE_DIR}/identity.json" ] && [ -z "$TOKEN_FROM" ]; then
		log "This node already has an identity: upgrading in place, no token needed."
	else
		[ -n "$TOKEN_FROM" ] || fatal "a join token is required for a first install. Fix: run 'levelrail-cli nodes add' and use --token-stdin or --token-file."
		read_token
		[ -n "$TOKEN" ] || fatal "the join token is empty."
	fi
	[ -n "$FINGERPRINT" ] || warn "no --ca-fingerprint: the first connection is not pinned. Copy it from the join command to pin it."
	ensure_docker
	if [ "$DRY" -eq 1 ]; then
		log "[dry-run] download ${AGENT}-linux-${GOARCH}${VERSION:+ ${VERSION}}, verify its checksum (and cosign signature when cosign is installed), install to ${BIN_PATH}"
	else
		tmpbin="$(mktemp)"
		trap 'rm -f "$tmpbin" "${tmpbin}.checksums" "${tmpbin}.checksums.sigstore.json"' EXIT
		fetch_binary "$tmpbin"
		install -m 0755 "$tmpbin" "$BIN_PATH"
	fi
	write_env
	if [ "$DRY" -eq 1 ]; then
		log "[dry-run] write ${UNIT_PATH}:"
		unit_text
	else
		unit_text >"$UNIT_PATH"
	fi
	act systemctl daemon-reload
	act systemctl enable "$AGENT"
	act systemctl restart "$AGENT"
	if wait_enrolled; then
		[ "$DRY" -eq 1 ] || scrub_token
		log "The agent is running and enrolled."
		log "Next: on the control plane run 'levelrail-cli nodes list' and look for this node as online."
		log "Logs: journalctl -u ${AGENT} -f   Remove: sh install-agent.sh --uninstall"
	else
		{
			log "The agent did not enroll within ${WAIT}s."
			log "Check: journalctl -u ${AGENT} -n 50 --no-pager"
			log "Likely fixes: the control plane is unreachable from here (open TCP 9443), the join token expired or was already used (mint a new one), or APP_AGENT_ADVERTISE_HOST on the control plane does not match --server."
		} >&2
		exit 1
	fi
}

if [ "$UNINSTALL" -eq 1 ]; then
	do_uninstall
else
	do_install
fi
