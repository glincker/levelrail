#!/bin/sh
# curl -fsSL https://levelrail.com/install.sh | sudo sh
#
# Installs the levelrail control plane binary, a systemd unit, and Docker
# (if missing) on a single Linux host.
#
# Usage: install.sh [install|upgrade|retain|uninstall] [--force] [--purge]
#   install    (default) preflight checks, then install or repair. Safe to re-run.
#   upgrade    replace the binary with a newer release and restart, keeping
#              the unit file and data. The previous binaries (last 3) are
#              kept in ${LEVELRAIL_INSTALL_DIR}/levelrail.releases for rollback.
#   retain     download and verify LEVELRAIL_VERSION and keep it beside the
#              installed binary, without installing it. Then run
#              `sudo levelrail rollback --to <version>` to switch to it.
#   uninstall  remove the service, unit, and binary. Data is kept unless
#              --purge is given.
#   --force    continue even if a preflight check fails.
#
# Env overrides:
#   LEVELRAIL_VERSION        release tag to install, e.g. v0.3.0
#   LEVELRAIL_CHANNEL        stable or beta. Unset means the newest stable
#                            release, falling back to the newest pre-release
#                            while no stable release exists.
#   LEVELRAIL_INSTALL_DIR    where the binary goes (default: /usr/local/bin)
#   LEVELRAIL_DATA_DIR       control plane data dir (default: /var/lib/levelrail-data)
#   LEVELRAIL_BINARY_FILE    install this local binary instead of downloading
#   LEVELRAIL_BINARY_URL     download the binary from this URL instead of a
#                            GitHub release (no checksum verification)
#   LEVELRAIL_SKIP_CHECKSUM=1  install even when checksums.txt is missing or
#                            does not list the binary (unverified; opt-out only)
#   APP_INSTALL_VERIFY       signature check on checksums.txt (cosign keyless):
#                            auto (default) verifies when cosign is installed
#                            and the release ships a signature; require fails
#                            unless the signature verifies; off skips it. The
#                            SHA-256 checksum is always verified.
#   LEVELRAIL_RELEASE_BASE_URL  download release assets from this https mirror
#                            directory instead of GitHub (LEVELRAIL_INSECURE_MIRROR=1
#                            permits http, for tests only)
#   LEVELRAIL_PUBLIC_IP      skip public IP discovery
#   LEVELRAIL_SKIP_REACHABILITY=1  skip the external port 80/443 self-test
#   LEVELRAIL_MIN_RAM_MB     RAM warning threshold (default: 1024)
#   LEVELRAIL_MIN_DISK_GB    free disk requirement (default: 10)
#   LEVELRAIL_MIN_DOCKER_MAJOR  minimum Docker major version (default: 24)
#   LEVELRAIL_HEALTH_WAIT    seconds to wait for the service (default: 60)
#   LEVELRAIL_CONFIGURE_UFW  set to 1 to allow SSH, 80/tcp, 443/tcp, and 443/udp in
#                            ufw and enable it if inactive. Off by default.
#   LEVELRAIL_DASHBOARD_PORT dashboard/API port (default: 8080). If taken and
#                            left at its default, the installer picks the
#                            next free port automatically; set this to pin one.
#   LEVELRAIL_HTTP_PORT      ingress HTTP port (default: 80)
#   LEVELRAIL_HTTPS_PORT     ingress HTTPS port (default: 443). Moving either
#                            ingress port off 80/443 disables automatic ACME
#                            TLS (Let's Encrypt only validates those two), so
#                            the installer never does this on its own; it
#                            fails preflight instead and tells you to set
#                            these explicitly once you've accepted that trade.
#   LEVELRAIL_SOCKET_ACTIVATION  systemd owns the ingress sockets, so a
#                            restart or upgrade of the control plane queues
#                            connections instead of refusing them (see
#                            docs/resilience.md). On by default for a new
#                            install; set 0 to opt out (the control plane
#                            then binds 80/443 itself). On upgrade, unset
#                            keeps the current mode, 1 switches an existing
#                            install over (one brief stop) and 0 switches
#                            it back.

set -eu

REPO="glincker/levelrail"
BINARY_NAME="levelrail"
SERVICE_NAME="levelrail"
INSTALL_DIR="${LEVELRAIL_INSTALL_DIR:-/usr/local/bin}"
DATA_DIR="${LEVELRAIL_DATA_DIR:-/var/lib/levelrail-data}"
UNIT_PATH="/etc/systemd/system/${SERVICE_NAME}.service"
SOCKET_ACTIVATION="${LEVELRAIL_SOCKET_ACTIVATION:-}"
HTTP_SOCKET_UNIT="${SERVICE_NAME}-http.socket"
HTTPS_SOCKET_UNIT="${SERVICE_NAME}-https.socket"
BIN_PATH="${INSTALL_DIR}/${BINARY_NAME}"
RELEASES_DIR="${LEVELRAIL_RELEASES_DIR:-${INSTALL_DIR}/${BINARY_NAME}.releases}"
DASHBOARD_PORT="${LEVELRAIL_DASHBOARD_PORT:-8080}"
DASHBOARD_PORT_PINNED=0
[ -z "${LEVELRAIL_DASHBOARD_PORT:-}" ] || DASHBOARD_PORT_PINNED=1
HTTP_PORT="${LEVELRAIL_HTTP_PORT:-80}"
HTTPS_PORT="${LEVELRAIL_HTTPS_PORT:-443}"
# The dashboard is plain HTTP until the operator configures a domain with TLS.
DASHBOARD_SCHEME="http"
HTTPS_ONLY="=https"
MIN_RAM_MB="${LEVELRAIL_MIN_RAM_MB:-1024}"
MIN_DISK_GB="${LEVELRAIL_MIN_DISK_GB:-10}"
MIN_DOCKER_MAJOR="${LEVELRAIL_MIN_DOCKER_MAJOR:-24}"
HEALTH_WAIT="${LEVELRAIL_HEALTH_WAIT:-60}"
VERIFY_MODE="${APP_INSTALL_VERIFY:-auto}"
COSIGN_IDENTITY_REGEXP="https://github.com/${REPO}/\\.github/workflows/release\\.yml@.*"
COSIGN_OIDC_ISSUER="https://token.actions.githubusercontent.com"

log() { printf '%s\n' "$*"; }
warn() { printf 'warning: %s\n' "$*" >&2; }
fatal() {
	printf 'error: %s\n' "$*" >&2
	exit 1
}

usage() {
	cat <<'EOF'
Usage: install.sh [install|upgrade|retain|uninstall] [--force] [--purge]
  install    (default) preflight checks, then install or repair
  upgrade    replace the binary with a newer release and restart
  retain     download and verify LEVELRAIL_VERSION and keep it for rollback
  uninstall  remove the service, unit, and binary (data kept unless --purge)
  --force    continue even if a preflight check fails
EOF
}

MODE="install"
FORCE=0
PURGE=0
for arg in "$@"; do
	case "$arg" in
	install | upgrade | retain | uninstall) MODE="$arg" ;;
	--force) FORCE=1 ;;
	--purge) PURGE=1 ;;
	-h | --help)
		usage
		exit 0
		;;
	*) fatal "unknown argument: $arg (see --help)" ;;
	esac
done
[ "$PURGE" -eq 0 ] || [ "$MODE" = "uninstall" ] || fatal "--purge only applies to uninstall"
case "$VERIFY_MODE" in
auto | require | off) ;;
*) fatal "APP_INSTALL_VERIFY must be auto, require or off, got: $VERIFY_MODE" ;;
esac

# These values land in a systemd unit and, for DATA_DIR, in rm -rf on purge.
safe_dir() {
	case "$2" in
	/ | /bin | /boot | /dev | /etc | /home | /lib | /lib64 | /opt | /proc | /root | /run | /sbin | /srv | /sys | /tmp | /usr | /usr/bin | /usr/local | /usr/local/bin | /var | /var/lib)
		if [ "$1" = "LEVELRAIL_DATA_DIR" ]; then fatal "$1 must not be a system directory: $2"; fi
		;;
	esac
	case "$2" in
	/*) ;;
	*) fatal "$1 must be an absolute path: $2" ;;
	esac
	case "$2" in
	*[!A-Za-z0-9._/-]* | *..*) fatal "$1 may only contain letters, digits, dot, dash, underscore and slash, and no '..': $2" ;;
	esac
}
safe_port() {
	case "$2" in
	'' | *[!0-9]*) fatal "$1 must be a port number: $2" ;;
	esac
	{ [ "$2" -ge 1 ] && [ "$2" -le 65535 ]; } || fatal "$1 must be between 1 and 65535: $2"
}
safe_dir LEVELRAIL_DATA_DIR "$DATA_DIR"
safe_dir LEVELRAIL_INSTALL_DIR "$INSTALL_DIR"
safe_port LEVELRAIL_DASHBOARD_PORT "$DASHBOARD_PORT"
safe_port LEVELRAIL_HTTP_PORT "$HTTP_PORT"
safe_port LEVELRAIL_HTTPS_PORT "$HTTPS_PORT"
case "${LEVELRAIL_VERSION:-}" in
*[!A-Za-z0-9._+-]*) fatal "LEVELRAIL_VERSION may only contain letters, digits, dot, dash, plus and underscore" ;;
esac

[ "$(id -u)" -eq 0 ] || fatal "must run as root, e.g.: curl -fsSL <url> | sudo sh"

github_api() {
	curl -fsSL --proto "$HTTPS_ONLY" --tlsv1.2 --connect-timeout 10 --max-time 30 \
		-H "Accept: application/vnd.github+json" "https://api.github.com/repos/${REPO}/$1" 2>/dev/null
}

# list_releases prints "<tag> <prerelease> <has-asset> <created-at>" per
# release, where has-asset is 1 when the release ships asset $1. Sorted by
# created_at here because the API's own list order is not reliable.
list_releases() {
	github_api "releases?per_page=30" | awk -v asset="\"name\": \"$1\"" '
		/"tag_name":/ {
			if (tag != "") print tag, pre, has, created
			tag = $0; sub(/.*"tag_name": *"/, "", tag); sub(/".*/, "", tag)
			pre = "false"; has = 0; created = ""
		}
		/"prerelease": *true/ { pre = "true" }
		/"created_at":/ && created == "" {
			created = $0
			sub(/.*"created_at": *"/, "", created); sub(/".*/, "", created)
		}
		index($0, asset) { has = 1 }
		END { if (tag != "") print tag, pre, has, created }' | sort -k4,4r
}

# resolve_version picks the newest release on the channel that actually
# ships a binary for this architecture, skipping releases with no assets.
resolve_version() {
	if [ -n "${LEVELRAIL_VERSION:-}" ]; then
		VERSION="$LEVELRAIL_VERSION"
		return
	fi
	channel="${LEVELRAIL_CHANNEL:-}"
	case "$channel" in
	"" | stable | beta) ;;
	*) fatal "LEVELRAIL_CHANNEL must be stable or beta, got: $channel" ;;
	esac
	asset="levelrail-linux-${GOARCH}"
	releases="$(list_releases "$asset" || true)"
	[ -n "$releases" ] || fatal "could not list releases from GitHub. Set LEVELRAIL_VERSION=vX.Y.Z to pin one."

	newest_stable="$(printf '%s\n' "$releases" | awk '$2 == "false" { print $1; exit }')"
	stable="$(printf '%s\n' "$releases" | awk '$2 == "false" && $3 == 1 { print $1; exit }')"
	newest_any="$(printf '%s\n' "$releases" | awk '{ print $1; exit }')"
	any="$(printf '%s\n' "$releases" | awk '$3 == 1 { print $1; exit }')"

	case "$channel" in
	stable)
		VERSION="$stable"
		newest="$newest_stable"
		;;
	beta)
		VERSION="$any"
		newest="$newest_any"
		;;
	*)
		VERSION="$stable"
		newest="$newest_stable"
		if [ -z "$VERSION" ]; then
			VERSION="$any"
			newest="$newest_any"
			[ -z "$VERSION" ] || log "No stable release with a ${asset} binary yet, using pre-release ${VERSION}."
		fi
		;;
	esac
	[ -n "$VERSION" ] || fatal "no ${channel:-published} release ships ${asset}. Set LEVELRAIL_VERSION=vX.Y.Z to pin one, or LEVELRAIL_BINARY_URL to a binary."
	if [ -n "$newest" ] && [ "$newest" != "$VERSION" ]; then
		warn "${newest} has no ${asset} binary, installing ${VERSION} instead"
	fi
}

detect_arch() {
	arch="$(uname -m)"
	case "$arch" in
	x86_64 | amd64) GOARCH="amd64" ;;
	aarch64 | arm64) GOARCH="arm64" ;;
	*) GOARCH="" ;;
	esac
}

# port_listening reports whether anything listens on TCP port $1, read from
# /proc so it works without ss or netstat installed.
port_listening() {
	hex="$(printf ':%04X' "$1")"
	for table in /proc/net/tcp /proc/net/tcp6; do
		[ -r "$table" ] || continue
		if awk -v p="$hex" 'NR > 1 && $4 == "0A" && substr($2, length($2) - 4) == p { found = 1 } END { exit !found }' "$table"; then
			return 0
		fi
	done
	return 1
}

service_active() {
	systemctl is-active --quiet "$SERVICE_NAME" 2>/dev/null ||
		systemctl is-active --quiet "$HTTP_SOCKET_UNIT" 2>/dev/null
}

# find_free_port scans start..start+19 and prints the first port
# port_listening reports free, or nothing if all 20 are taken.
find_free_port() {
	start="$1"
	i=0
	while [ "$i" -lt 20 ]; do
		candidate=$((start + i))
		if ! port_listening "$candidate"; then
			printf '%s' "$candidate"
			return 0
		fi
		i=$((i + 1))
	done
	return 1
}

PREFLIGHT_FAILS=0
row() {
	printf '  %-10s %-5s %s\n' "$1" "$2" "$3"
	[ "$2" != "FAIL" ] || PREFLIGHT_FAILS=$((PREFLIGHT_FAILS + 1))
}

docker_major() {
	v="$(docker version --format '{{.Server.Version}}' 2>/dev/null || true)"
	[ -n "$v" ] || v="$(docker --version 2>/dev/null | sed -E 's/^[^0-9]*([0-9]+).*/\1/' || true)"
	printf '%s' "${v%%.*}"
}

first_existing_dir() {
	d="$1"
	while [ ! -d "$d" ]; do d="$(dirname "$d")"; done
	printf '%s' "$d"
}

preflight() {
	log "Preflight checks:"
	row root ok "running as root"

	detect_arch
	if [ -n "$GOARCH" ]; then row arch ok "$(uname -m)"; else row arch FAIL "$(uname -m) (supported: x86_64, aarch64)"; fi

	os="$(uname -s)"
	distro=""
	# shellcheck disable=SC1091
	if [ -r /etc/os-release ]; then distro="$(. /etc/os-release && printf '%s' "${PRETTY_NAME:-}")"; fi
	if [ "$os" = "Linux" ]; then row os ok "${distro:-Linux}"; else row os FAIL "$os (Linux only)"; fi

	if command -v systemctl >/dev/null 2>&1 && [ -d /run/systemd/system ]; then
		row systemd ok "running"
	else
		row systemd FAIL "systemd is required as the init system"
	fi

	if command -v curl >/dev/null 2>&1; then row curl ok "found"; else row curl FAIL "curl is required"; fi

	if command -v docker >/dev/null 2>&1; then
		major="$(docker_major)"
		case "$major" in
		'' | *[!0-9]*) row docker warn "installed, version unknown" ;;
		*) if [ "$major" -ge "$MIN_DOCKER_MAJOR" ]; then row docker ok "version $major"; else row docker FAIL "version $major, need >= $MIN_DOCKER_MAJOR"; fi ;;
		esac
	else
		row docker ok "not found, will install via get.docker.com"
	fi

	mem_kb="$(awk '/^MemTotal:/ { print $2 }' /proc/meminfo 2>/dev/null || echo 0)"
	mem_mb=$((${mem_kb:-0} / 1024))
	if [ "$mem_mb" -ge "$MIN_RAM_MB" ]; then row ram ok "${mem_mb} MB"; else row ram warn "${mem_mb} MB, ${MIN_RAM_MB} MB or more recommended"; fi

	disk_dir="$(first_existing_dir "$DATA_DIR")"
	free_kb="$(df -Pk "$disk_dir" 2>/dev/null | awk 'NR == 2 { print $4 }')"
	free_gb=$((${free_kb:-0} / 1024 / 1024))
	if [ "$free_gb" -ge "$MIN_DISK_GB" ]; then row disk ok "${free_gb} GB free on ${disk_dir}"; else row disk FAIL "${free_gb} GB free on ${disk_dir}, need ${MIN_DISK_GB} GB"; fi

	for port in "$HTTP_PORT" "$HTTPS_PORT"; do
		if ! port_listening "$port"; then
			row "port $port" ok "free"
		elif service_active; then
			row "port $port" ok "in use, presumably by ${SERVICE_NAME} (reinstall)"
		else
			row "port $port" FAIL "already in use by another process (automatic TLS needs 80/443; set LEVELRAIL_HTTP_PORT/LEVELRAIL_HTTPS_PORT to run on alternate ports instead, accepting that automatic ACME certs won't work)"
		fi
	done

	if ! port_listening "$DASHBOARD_PORT"; then
		row "port $DASHBOARD_PORT" ok "free"
	elif service_active; then
		row "port $DASHBOARD_PORT" ok "in use, presumably by ${SERVICE_NAME} (reinstall)"
	elif [ "$DASHBOARD_PORT_PINNED" -eq 0 ] && free_port="$(find_free_port "$DASHBOARD_PORT")"; then
		row "port $DASHBOARD_PORT" ok "taken, using $free_port instead"
		DASHBOARD_PORT="$free_port"
	else
		row "port $DASHBOARD_PORT" FAIL "already in use by another process"
	fi

	if [ "$PREFLIGHT_FAILS" -gt 0 ]; then
		[ "$FORCE" -eq 1 ] || fatal "${PREFLIGHT_FAILS} preflight check(s) failed. Fix them, or re-run with --force to continue anyway."
		warn "${PREFLIGHT_FAILS} preflight check(s) failed, continuing because of --force"
	fi
	[ -n "$GOARCH" ] || fatal "cannot continue on an unsupported architecture"
}

ensure_docker() {
	if ! command -v docker >/dev/null 2>&1; then
		log "Installing Docker via get.docker.com..."
		curl -fsSL --proto "$HTTPS_ONLY" --tlsv1.2 --connect-timeout 10 --max-time 120 https://get.docker.com | sh
	fi
	systemctl enable --now docker
}

fetch_binary() {
	dest="$1"
	if [ -n "${LEVELRAIL_BINARY_FILE:-}" ]; then
		[ -f "$LEVELRAIL_BINARY_FILE" ] || fatal "LEVELRAIL_BINARY_FILE not found: $LEVELRAIL_BINARY_FILE"
		log "Using local binary ${LEVELRAIL_BINARY_FILE}."
		cp "$LEVELRAIL_BINARY_FILE" "$dest"
		VERSION="${LEVELRAIL_VERSION:-local build}"
		return
	fi
	if [ -n "${LEVELRAIL_BINARY_URL:-}" ]; then
		log "Downloading ${LEVELRAIL_BINARY_URL}..."
		curl -fsSL --connect-timeout 10 --max-time 300 -o "$dest" "$LEVELRAIL_BINARY_URL" || fatal "download failed: $LEVELRAIL_BINARY_URL"
		VERSION="${LEVELRAIL_VERSION:-custom build}"
		return
	fi

	resolve_version
	asset="levelrail-linux-${GOARCH}"
	base_url="${LEVELRAIL_RELEASE_BASE_URL:-https://github.com/${REPO}/releases/download/${VERSION}}"
	proto="$HTTPS_ONLY"
	case "$base_url" in
	https://*) ;;
	http://*)
		[ "${LEVELRAIL_INSECURE_MIRROR:-}" = "1" ] || fatal "LEVELRAIL_RELEASE_BASE_URL must be https (LEVELRAIL_INSECURE_MIRROR=1 allows http for tests)"
		proto="=http"
		;;
	*) fatal "LEVELRAIL_RELEASE_BASE_URL must start with https://" ;;
	esac
	log "Downloading ${asset} ${VERSION}..."
	curl -fsSL --proto "$proto" --tlsv1.2 --connect-timeout 10 --max-time 300 -o "$dest" "${base_url}/${asset}" ||
		fatal "download failed: ${base_url}/${asset}. The release may not ship this binary; pick another with LEVELRAIL_VERSION."

	sums="${dest}.checksums"
	if curl -fsSL --proto "$proto" --tlsv1.2 --connect-timeout 10 --max-time 60 -o "$sums" "${base_url}/checksums.txt" 2>/dev/null; then
		verify_signature "$sums" "$base_url" "$proto"
		expected="$(grep " ${asset}\$" "$sums" | awk '{ print $1 }')"
		if [ -n "$expected" ]; then
			actual="$(sha256sum "$dest" | awk '{ print $1 }')"
			[ "$expected" = "$actual" ] || fatal "checksum mismatch for ${asset}: expected ${expected}, got ${actual}"
			log "Checksum verified."
		else
			checksum_unavailable "${asset} is not listed in checksums.txt for ${VERSION}"
		fi
	else
		checksum_unavailable "no checksums.txt published for ${VERSION}"
	fi
}

# verify_signature checks the cosign keyless bundle over checksums.txt. A
# failed verification is always fatal; only a missing cosign or bundle is
# tolerated, and never under APP_INSTALL_VERIFY=require.
verify_signature() {
	sums="$1"
	base_url="$2"
	proto="$3"
	[ "$VERIFY_MODE" != "off" ] || return 0
	if ! command -v cosign >/dev/null 2>&1; then
		[ "$VERIFY_MODE" != "require" ] || fatal "APP_INSTALL_VERIFY=require but cosign is not installed. Install it from https://docs.sigstore.dev/cosign/system_config/installation/ or unset APP_INSTALL_VERIFY."
		log "cosign not found: verifying the SHA-256 checksum only (install cosign to also verify the release signature)."
		return 0
	fi
	bundle="${sums}.sigstore.json"
	if ! curl -fsSL --proto "$proto" --tlsv1.2 --connect-timeout 10 --max-time 60 -o "$bundle" "${base_url}/checksums.txt.sigstore.json" 2>/dev/null; then
		[ "$VERIFY_MODE" != "require" ] || fatal "APP_INSTALL_VERIFY=require but ${VERSION} publishes no checksums.txt.sigstore.json"
		warn "${VERSION} publishes no signature for checksums.txt, verifying the SHA-256 checksum only"
		return 0
	fi
	cosign verify-blob --bundle "$bundle" \
		--certificate-identity-regexp "$COSIGN_IDENTITY_REGEXP" \
		--certificate-oidc-issuer "$COSIGN_OIDC_ISSUER" "$sums" >/dev/null 2>&1 ||
		fatal "cosign signature verification of checksums.txt failed for ${VERSION}, refusing to install"
	log "Signature verified (cosign, keyless)."
}

checksum_unavailable() {
	if [ "$VERIFY_MODE" = "require" ]; then
		fatal "$1, and APP_INSTALL_VERIFY=require does not allow an unverified install"
	fi
	if [ "${LEVELRAIL_SKIP_CHECKSUM:-}" = "1" ]; then
		warn "$1, skipping verification because LEVELRAIL_SKIP_CHECKSUM=1"
		return 0
	fi
	fatal "$1, refusing to install an unverified binary. Pick another release with LEVELRAIL_VERSION, or set LEVELRAIL_SKIP_CHECKSUM=1 to install without verification."
}

# supports_retain is true when the binary at $1 has the retain subcommand. It
# greps for the usage text rather than running the binary: an older release
# would ignore an unknown argument and start serving.
supports_retain() {
	[ -f "$1" ] && grep -aq 'retain --version <tag> --file <binary>' "$1"
}

# retain_releases keeps the previous and the new binary for rollback. It never
# fails an install: rollback is a convenience, not a precondition.
retain_releases() {
	previous="$1"
	supports_retain "$BIN_PATH" || return 0
	mkdir -p "$RELEASES_DIR" 2>/dev/null || return 0
	marker="${RELEASES_DIR}/.current"
	if [ -n "$previous" ] && [ -f "$marker" ]; then
		old="$(cat "$marker")"
		"$BIN_PATH" retain --version "$old" --file "$previous" --releases-dir "$RELEASES_DIR" >/dev/null 2>&1 ||
			warn "could not keep the previous binary (${old}) for rollback"
	fi
	case "$VERSION" in
	v[0-9]*)
		if "$BIN_PATH" retain --version "$VERSION" --file "$BIN_PATH" --releases-dir "$RELEASES_DIR" >/dev/null 2>&1; then
			printf '%s\n' "$VERSION" >"$marker"
		else
			warn "could not keep ${VERSION} for rollback"
		fi
		;;
	esac
}

install_binary() {
	tmp_dir="$(mktemp -d)"
	trap 'rm -rf "$tmp_dir"' EXIT
	fetch_binary "$tmp_dir/$BINARY_NAME"
	mkdir -p "$INSTALL_DIR"
	previous=""
	if [ -x "$BIN_PATH" ]; then
		cp -p "$BIN_PATH" "$tmp_dir/previous"
		previous="$tmp_dir/previous"
	fi
	install -m 0755 "$tmp_dir/$BINARY_NAME" "$BIN_PATH"
	retain_releases "$previous"
}

write_brand() {
	mkdir -p "$DATA_DIR"
	chmod 0750 "$DATA_DIR"
	[ ! -f "$DATA_DIR/brand.yaml" ] || return 0
	cat >"$DATA_DIR/brand.yaml" <<-EOF
		name: Levelrail
		short_name: Levelrail
		binary_name: levelrail
		domain: glinr.com/levelrail
		support_url: https://github.com/GLINCKER/levelrail/issues
		docs_url: https://levelrail.glinr.com
		primary_color: ""
		logo_svg: ""
	EOF
}

write_socket_units() {
	cat >"/etc/systemd/system/${HTTP_SOCKET_UNIT}" <<EOF
[Unit]
Description=Levelrail ingress HTTP socket (held open across control plane restarts)

[Socket]
ListenStream=${HTTP_PORT}
FileDescriptorName=http
Service=${SERVICE_NAME}.service

[Install]
WantedBy=sockets.target
EOF
	cat >"/etc/systemd/system/${HTTPS_SOCKET_UNIT}" <<EOF
[Unit]
Description=Levelrail ingress HTTPS socket (held open across control plane restarts)

[Socket]
ListenStream=${HTTPS_PORT}
FileDescriptorName=https
Service=${SERVICE_NAME}.service

[Install]
WantedBy=sockets.target
EOF
}

# remove_socket_units goes back to the control plane binding 80/443 itself.
remove_socket_units() {
	systemctl disable --now "$HTTP_SOCKET_UNIT" "$HTTPS_SOCKET_UNIT" >/dev/null 2>&1 || true
	rm -f "/etc/systemd/system/${HTTP_SOCKET_UNIT}" "/etc/systemd/system/${HTTPS_SOCKET_UNIT}"
}

write_unit() {
	socket_deps=""
	if [ "$SOCKET_ACTIVATION" = "1" ] || [ -f "/etc/systemd/system/${HTTP_SOCKET_UNIT}" ]; then
		socket_deps="Requires=${HTTP_SOCKET_UNIT} ${HTTPS_SOCKET_UNIT}
After=${HTTP_SOCKET_UNIT} ${HTTPS_SOCKET_UNIT}"
	fi
	cat >"$UNIT_PATH" <<EOF
[Unit]
Description=Levelrail control plane
After=network-online.target docker.service
Requires=docker.service
Wants=network-online.target
${socket_deps}

[Service]
ExecStart=${BIN_PATH}
WorkingDirectory=${DATA_DIR}
Environment=APP_DATA_DIR=${DATA_DIR}
Environment=APP_HTTP_ADDR=:${DASHBOARD_PORT}
Environment=APP_INGRESS_HTTP_ADDR=:${HTTP_PORT}
Environment=APP_INGRESS_HTTPS_ADDR=:${HTTPS_PORT}
Restart=on-failure
RestartSec=5

[Install]
WantedBy=multi-user.target
EOF
}

# configure_ufw allows SSH before anything else and only enables ufw if it
# was not already active, so it can never lock the operator out.
configure_ufw() {
	[ "${LEVELRAIL_CONFIGURE_UFW:-0}" = "1" ] || return 0
	if ! command -v ufw >/dev/null 2>&1; then
		log "LEVELRAIL_CONFIGURE_UFW=1 set, but ufw is not installed, skipping."
		return 0
	fi
	log "Configuring ufw (LEVELRAIL_CONFIGURE_UFW=1)..."
	was_active=0
	ufw status 2>/dev/null | grep -q "^Status: active" && was_active=1
	ufw allow OpenSSH >/dev/null 2>&1 || ufw allow 22/tcp >/dev/null 2>&1
	ufw allow "${HTTP_PORT}/tcp" >/dev/null 2>&1
	ufw allow "${HTTPS_PORT}/tcp" >/dev/null 2>&1
	ufw allow "${HTTPS_PORT}/udp" >/dev/null 2>&1
	if [ "$was_active" -eq 1 ]; then
		log "ufw was already active, added rules without re-enabling."
	else
		ufw --force enable >/dev/null 2>&1
		log "ufw enabled: SSH, ${HTTP_PORT}/tcp, ${HTTPS_PORT}/tcp, and ${HTTPS_PORT}/udp (HTTP/3) are allowed, everything else denied by default."
	fi
}

wait_healthy() {
	log "Waiting for the control plane to come up..."
	waited=0
	while [ "$waited" -lt "$HEALTH_WAIT" ]; do
		if curl -fsS --max-time 3 -o /dev/null "http://127.0.0.1:${DASHBOARD_PORT}/healthz" 2>/dev/null; then
			log "Control plane is healthy."
			return 0
		fi
		sleep 2
		waited=$((waited + 2))
	done
	fatal "control plane did not become healthy within ${HEALTH_WAIT}s. Check: systemctl status ${SERVICE_NAME} && journalctl -u ${SERVICE_NAME} -n 100 --no-pager"
}

discover_public_ip() {
	PUBLIC_IP="${LEVELRAIL_PUBLIC_IP:-}"
	[ -z "$PUBLIC_IP" ] || return 0
	for svc in https://api.ipify.org https://ifconfig.me/ip; do
		PUBLIC_IP="$(curl -4 -fsS --proto "$HTTPS_ONLY" --connect-timeout 3 --max-time 5 "$svc" 2>/dev/null | tr -d '[:space:]' || true)"
		case "$PUBLIC_IP" in
		*[!0-9.]* | "") PUBLIC_IP="" ;;
		*) return 0 ;;
		esac
	done
}

# start_temp_listener serves port $1 for at most 20s so the reachability
# probe has something to connect to when nothing else listens there yet.
TEMP_PID=""
start_temp_listener() {
	TEMP_PID=""
	command -v python3 >/dev/null 2>&1 || return 1
	command -v timeout >/dev/null 2>&1 || return 1
	timeout 20 python3 -m http.server "$1" --bind 0.0.0.0 >/dev/null 2>&1 &
	TEMP_PID=$!
	tries=0
	while [ "$tries" -lt 10 ]; do
		port_listening "$1" && return 0
		sleep 0.3 2>/dev/null || sleep 1
		tries=$((tries + 1))
	done
	return 1
}

stop_temp_listener() {
	[ -n "$TEMP_PID" ] || return 0
	kill "$TEMP_PID" 2>/dev/null || true
	wait "$TEMP_PID" 2>/dev/null || true
	TEMP_PID=""
}

# reachability_test probes 80/443 on the public IP, the way a visitor or an
# ACME server would. Curl exit 7 (refused) or 28 (timeout) means blocked;
# anything else, TLS errors included, proves the TCP port is reachable.
reachability_test() {
	[ "${LEVELRAIL_SKIP_REACHABILITY:-0}" != "1" ] || return 0
	if [ "$HTTP_PORT" != "80" ] || [ "$HTTPS_PORT" != "443" ]; then
		log "Ingress is on ${HTTP_PORT}/${HTTPS_PORT}, not 80/443, so automatic ACME TLS is already off; skipping the reachability test."
		return 0
	fi
	if [ -z "$PUBLIC_IP" ]; then
		warn "could not determine this server's public IP, skipping the port 80/443 reachability test"
		return 0
	fi
	log "Checking that ports 80 and 443 are reachable at ${PUBLIC_IP}..."
	blocked=""
	for port in "$HTTP_PORT" "$HTTPS_PORT"; do
		scheme="http"
		[ "$port" != "443" ] || scheme="https"
		if ! port_listening "$port" && ! start_temp_listener "$port"; then
			log "  port $port: skipped (nothing listening and no python3 for a temporary listener)"
			continue
		fi
		rc=0
		curl -k -s -o /dev/null --connect-timeout 4 --max-time 8 "${scheme}://${PUBLIC_IP}:${port}/" || rc=$?
		stop_temp_listener
		case "$rc" in
		7 | 28)
			log "  port $port: NOT reachable"
			blocked="$blocked $port"
			;;
		*) log "  port $port: reachable" ;;
		esac
	done
	[ -n "$blocked" ] || return 0
	warn "port(s)${blocked} did not answer on ${PUBLIC_IP}. Apps and TLS certificates need 80 and 443 open."
	cat <<EOF
  If this server runs a host firewall, open the ports:
    ufw:        sudo ufw allow 80/tcp && sudo ufw allow 443/tcp
    firewalld:  sudo firewall-cmd --permanent --add-service=http --add-service=https && sudo firewall-cmd --reload
  Also check your cloud provider's firewall or security group.
  Some providers never route a server's own public IP back to itself, so
  this check can fail even when the ports are open to the internet.
EOF
}

# local_ips lists this host's global addresses, skipping container bridges
# and virtual interfaces, which are unreachable from anywhere useful.
local_ips() {
	if command -v ip >/dev/null 2>&1; then
		ip -o addr show scope global 2>/dev/null | while read -r _ ifname fam addr _; do
			case "$ifname" in docker* | br-* | veth* | cni* | flannel* | virbr* | lo) continue ;; esac
			case "$fam" in inet) printf '%s\n' "${addr%%/*}" ;; esac
		done
		return 0
	fi
	for ip in $(hostname -I 2>/dev/null || true); do
		case "$ip" in *:* | 172.1[6-9].* | 172.2[0-9].* | 172.3[01].*) continue ;; esac
		printf '%s\n' "$ip"
	done
}

dashboard_link() {
	case "$1" in *:*) host="[$1]" ;; *) host="$1" ;; esac
	if [ -n "$token" ]; then
		printf '%s://%s:%s/login?setup=%s' "$DASHBOARD_SCHEME" "$host" "$DASHBOARD_PORT" "$token"
	else
		printf '%s://%s:%s' "$DASHBOARD_SCHEME" "$host" "$DASHBOARD_PORT"
	fi
}

print_summary() {
	token=""
	[ ! -r "$DATA_DIR/setup-token" ] || token="$(tr -d '[:space:]' <"$DATA_DIR/setup-token")"

	log ""
	log "Levelrail ${VERSION} installed and running."
	log ""
	log "Open the dashboard:"
	if [ -n "$PUBLIC_IP" ]; then
		log "  $(dashboard_link "$PUBLIC_IP")"
		others="$(local_ips | grep -vxF "$PUBLIC_IP" || true)"
		if [ -n "$others" ]; then
			log ""
			log "Private network address (only reachable from inside that network):"
			for ip in $others; do log "  $(dashboard_link "$ip")"; done
		fi
	else
		found=0
		for ip in $(local_ips); do
			log "  $(dashboard_link "$ip")"
			found=1
		done
		[ "$found" -eq 1 ] || log "  ${DASHBOARD_SCHEME}://<server-ip>:${DASHBOARD_PORT}"
	fi
	log ""
	if [ -n "$token" ]; then
		cat <<EOF
Create the first admin account with this one-time setup token:
  ${token}
The links above pre-fill it. Print it again any time with:
  sudo APP_DATA_DIR=${DATA_DIR} ${BIN_PATH} setup-token

EOF
	fi
	cat <<EOF
IMPORTANT: back up ${DATA_DIR}/master.key somewhere safe. Every stored
secret is encrypted with it, and it cannot be recovered if lost.

The dashboard is served over plain HTTP until you enable HTTPS.

EOF
	sslip_hint
	cat <<EOF

Manage it from your own machine with the CLI (no root needed there):
  curl -fsSL https://levelrail.com/install-cli.sh | sh
  export APP_API_URL=${DASHBOARD_SCHEME}://<server-ip>:${DASHBOARD_PORT}
  levelrail-cli auth login --device

  Service status: systemctl status ${SERVICE_NAME}
  Logs:           journalctl -u ${SERVICE_NAME} -f
  Data directory: ${DATA_DIR}
  Upgrade:        curl -fsSL https://raw.githubusercontent.com/${REPO}/main/install.sh | sudo sh -s upgrade
  Uninstall:      curl -fsSL https://raw.githubusercontent.com/${REPO}/main/install.sh | sudo sh -s uninstall
  Locked out:     sudo APP_DATA_DIR=${DATA_DIR} ${BIN_PATH} recover-admin --username admin
EOF
}

# sslip_hint explains the zero-DNS HTTPS path when the public address is a
# plain IPv4: <dashed-ip>.sslip.io already resolves to it.
sslip_hint() {
	case "$PUBLIC_IP" in "" | *[!0-9.]*) return 0 ;; esac
	dashed="$(printf '%s' "$PUBLIC_IP" | tr '.' '-')"
	cat <<EOF
Free HTTPS with no DNS setup: https://${dashed}.sslip.io already points at
this server. In the dashboard, open Domains and click "Enable HTTPS" to get
a real Let's Encrypt certificate for it (ports 80 and 443 must be open), or:
  levelrail-cli settings ingress https enable --email you@example.com
Until then that https address shows a browser certificate warning. That is
expected, not an attack: keep using the http link above until it is enabled.
EOF
}

# write_upgrade_marker leaves who ran this and how for the control plane's
# next boot, which records it in upgrade history and deletes the file. Never
# fatal: history is a convenience, not a precondition.
write_upgrade_marker() {
	method="$1"
	who="${SUDO_USER:-$(id -un 2>/dev/null || echo unknown)}"
	who="$(printf '%s' "$who" | tr -cd 'A-Za-z0-9._@-')"
	chan="$(printf '%s' "${LEVELRAIL_CHANNEL:-}" | tr -cd '[:lower:]')"
	when="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
	mkdir -p "$DATA_DIR" 2>/dev/null || return 0
	(
		umask 077
		printf '{"to_version":"%s","initiator":"%s","method":"%s","channel":"%s","written_at":"%s"}\n' \
			"$VERSION" "${who:-unknown}" "$method" "$chan" "$when" >"$DATA_DIR/upgrade-context.json.tmp" &&
			mv "$DATA_DIR/upgrade-context.json.tmp" "$DATA_DIR/upgrade-context.json"
	) 2>/dev/null || warn "could not record who ran this upgrade for the upgrade history"
}

do_install() {
	[ -n "$SOCKET_ACTIVATION" ] || SOCKET_ACTIVATION=1
	preflight
	ensure_docker
	install_binary
	write_brand
	write_unit
	[ "$SOCKET_ACTIVATION" != "1" ] || write_socket_units
	configure_ufw
	systemctl daemon-reload
	systemctl enable "$SERVICE_NAME" >/dev/null 2>&1
	if [ "$SOCKET_ACTIVATION" = "1" ]; then
		systemctl enable --now "$HTTP_SOCKET_UNIT" "$HTTPS_SOCKET_UNIT" >/dev/null 2>&1
	fi
	write_upgrade_marker "install.sh install"
	systemctl restart "$SERVICE_NAME"
	wait_healthy
	discover_public_ip
	reachability_test
	print_summary
}

do_upgrade() {
	[ -x "$BIN_PATH" ] || fatal "${BIN_PATH} not found, nothing to upgrade. Run the installer without arguments first."
	detect_arch
	[ -n "$GOARCH" ] || fatal "unsupported architecture: $(uname -m)"
	install_binary
	case "$SOCKET_ACTIVATION" in
	1)
		write_unit
		write_socket_units
		systemctl daemon-reload
		systemctl stop "$SERVICE_NAME" >/dev/null 2>&1 || true
		systemctl enable --now "$HTTP_SOCKET_UNIT" "$HTTPS_SOCKET_UNIT" >/dev/null 2>&1
		;;
	0)
		remove_socket_units
		write_unit
		;;
	*) [ -f "$UNIT_PATH" ] || write_unit ;;
	esac
	systemctl daemon-reload
	write_upgrade_marker "install.sh upgrade"
	systemctl restart "$SERVICE_NAME"
	wait_healthy
	log "Upgraded to ${VERSION}."
}

do_retain() {
	[ -x "$BIN_PATH" ] || fatal "${BIN_PATH} not found. Run the installer without arguments first."
	supports_retain "$BIN_PATH" || fatal "the installed ${BINARY_NAME} predates release retention. Upgrade it first (install.sh upgrade), then retain."
	[ -n "${LEVELRAIL_VERSION:-}" ] || fatal "set LEVELRAIL_VERSION=vX.Y.Z to the release you want to roll back to"
	detect_arch
	[ -n "$GOARCH" ] || fatal "unsupported architecture: $(uname -m)"
	tmp_dir="$(mktemp -d)"
	trap 'rm -rf "$tmp_dir"' EXIT
	fetch_binary "$tmp_dir/$BINARY_NAME"
	schema=-1
	if [ -z "${LEVELRAIL_BINARY_FILE:-}${LEVELRAIL_BINARY_URL:-}" ] &&
		curl -fsSL --proto "$proto" --tlsv1.2 --connect-timeout 10 --max-time 30 -o "$tmp_dir/manifest.json" "${base_url}/release-manifest.json" 2>/dev/null; then
		found="$(sed -n 's/.*"schema_version": *\([0-9][0-9]*\).*/\1/p' "$tmp_dir/manifest.json" | head -n 1)"
		[ -z "$found" ] || schema="$found"
	fi
	mkdir -p "$RELEASES_DIR"
	"$BIN_PATH" retain --version "$VERSION" --file "$tmp_dir/$BINARY_NAME" --schema-version "$schema" --releases-dir "$RELEASES_DIR"
	log "Next, on this host: sudo ${BINARY_NAME} rollback --to ${VERSION}"
}

do_uninstall() {
	if [ -f "$UNIT_PATH" ]; then
		systemctl disable --now "$SERVICE_NAME" >/dev/null 2>&1 || true
		remove_socket_units
		rm -f "$UNIT_PATH"
		systemctl daemon-reload
	fi
	rm -f "$BIN_PATH"
	log "Removed the ${SERVICE_NAME} service and ${BIN_PATH}."
	if [ "$PURGE" -eq 1 ]; then
		case "$DATA_DIR" in
		"" | / | /var | /var/lib | /usr | /etc | /home | /root) fatal "refusing to delete suspicious data dir: '$DATA_DIR'" ;;
		esac
		rm -rf "$DATA_DIR"
		log "Deleted ${DATA_DIR}."
	else
		log "Kept ${DATA_DIR} (database, master key, certificates). Re-run with --purge to delete it."
	fi
	log "Docker and any app containers it runs were left untouched."
}

case "$MODE" in
install) do_install ;;
upgrade) do_upgrade ;;
retain) do_retain ;;
uninstall) do_uninstall ;;
esac
