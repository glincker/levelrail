#!/bin/sh
# curl -fsSL https://levelrail.com/install-cli.sh | sh
#
# Installs levelrail-cli, the client for an already-running Levelrail
# control plane. This is the client, not the server: it needs no root,
# no systemd, no Docker. See install.sh for the server.
#
# Usage: install-cli.sh [--force]
#   --force   continue even if an existing levelrail-cli looks newer
#
# Env overrides:
#   LEVELRAIL_VERSION        release tag to install, e.g. v0.3.0
#   LEVELRAIL_CHANNEL        stable or beta, same meaning as install.sh
#   LEVELRAIL_CLI_INSTALL_DIR  where the binary goes (default: $HOME/.local/bin,
#                            no root needed; set to /usr/local/bin and run
#                            with sudo for a system-wide install instead)
#   LEVELRAIL_BINARY_FILE    install this local binary instead of downloading
#   LEVELRAIL_SKIP_CHECKSUM=1  install even when checksums.txt is missing or
#                            does not list the binary (unverified; opt-out only)
#   APP_INSTALL_VERIFY       signature check on checksums.txt (cosign keyless),
#                            same meaning as install.sh: auto, require, or off
#   LEVELRAIL_RELEASE_BASE_URL  download release assets from this https mirror
#                            instead of GitHub

set -eu

REPO="glincker/levelrail"
BINARY_NAME="levelrail-cli"
INSTALL_DIR="${LEVELRAIL_CLI_INSTALL_DIR:-${HOME}/.local/bin}"
BIN_PATH="${INSTALL_DIR}/${BINARY_NAME}"
HTTPS_ONLY="=https"
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
Usage: install-cli.sh [--force]
  --force   continue even if an existing levelrail-cli looks newer
EOF
}

FORCE=0
for arg in "$@"; do
	case "$arg" in
	--force) FORCE=1 ;;
	-h | --help)
		usage
		exit 0
		;;
	*) fatal "unknown argument: $arg" ;;
	esac
done

detect_platform() {
	os="$(uname -s)"
	case "$os" in
	Linux) GOOS="linux" ;;
	Darwin) GOOS="darwin" ;;
	*) fatal "unsupported OS: ${os}. Windows isn't published yet, run this from WSL." ;;
	esac
	arch="$(uname -m)"
	case "$arch" in
	x86_64 | amd64) GOARCH="amd64" ;;
	aarch64 | arm64) GOARCH="arm64" ;;
	*) fatal "unsupported architecture: ${arch}" ;;
	esac
}

github_api() {
	curl -fsSL --proto "$HTTPS_ONLY" --tlsv1.2 --connect-timeout 10 --max-time 30 \
		-H "Accept: application/vnd.github+json" "https://api.github.com/repos/${REPO}/$1" 2>/dev/null
}

# Same shape as install.sh's list_releases/resolve_version: picks the
# newest release on the channel that actually ships this platform's CLI
# asset, skipping releases with no assets yet.
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
	asset="${BINARY_NAME}-${GOOS}-${GOARCH}"
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
	[ -n "$VERSION" ] || fatal "no ${channel:-published} release ships ${asset}. Set LEVELRAIL_VERSION=vX.Y.Z to pin one."
	if [ -n "$newest" ] && [ "$newest" != "$VERSION" ]; then
		warn "${newest} has no ${asset} binary, installing ${VERSION} instead"
	fi
}

# Same cosign keyless check install.sh runs, over the same checksums.txt
# a release publishes: the CLI binaries are checksummed and signed
# alongside the server binary by the one release workflow, nothing extra
# to set up here.
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

fetch_binary() {
	dest="$1"
	if [ -n "${LEVELRAIL_BINARY_FILE:-}" ]; then
		[ -f "$LEVELRAIL_BINARY_FILE" ] || fatal "LEVELRAIL_BINARY_FILE not found: $LEVELRAIL_BINARY_FILE"
		log "Using local binary ${LEVELRAIL_BINARY_FILE}."
		cp "$LEVELRAIL_BINARY_FILE" "$dest"
		VERSION="${LEVELRAIL_VERSION:-local build}"
		return
	fi

	resolve_version
	asset="${BINARY_NAME}-${GOOS}-${GOARCH}"
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
	curl -fsSL --proto "$proto" --tlsv1.2 --connect-timeout 10 --max-time 120 -o "$dest" "${base_url}/${asset}" ||
		fatal "download failed: ${base_url}/${asset}. The release may not ship this binary; pick another with LEVELRAIL_VERSION."

	sums="${dest}.checksums"
	if curl -fsSL --proto "$proto" --tlsv1.2 --connect-timeout 10 --max-time 60 -o "$sums" "${base_url}/checksums.txt" 2>/dev/null; then
		verify_signature "$sums" "$base_url" "$proto"
		expected="$(grep " ${asset}\$" "$sums" | awk '{ print $1 }')"
		if [ -n "$expected" ]; then
			actual="$(sha256sum "$dest" 2>/dev/null | awk '{ print $1 }')"
			[ -n "$actual" ] || actual="$(shasum -a 256 "$dest" | awk '{ print $1 }')"
			[ "$expected" = "$actual" ] || fatal "checksum mismatch for ${asset}: expected ${expected}, got ${actual}"
			log "Checksum verified."
		else
			checksum_unavailable "${asset} is not listed in checksums.txt for ${VERSION}"
		fi
	else
		checksum_unavailable "no checksums.txt published for ${VERSION}"
	fi
}

main() {
	detect_platform
	tmp_dir="$(mktemp -d)"
	trap 'rm -rf "$tmp_dir"' EXIT
	tmp_bin="${tmp_dir}/${BINARY_NAME}"

	if [ -x "$BIN_PATH" ] && [ "$FORCE" -eq 0 ] && [ -z "${LEVELRAIL_VERSION:-}" ] && [ -z "${LEVELRAIL_BINARY_FILE:-}" ]; then
		current="$("$BIN_PATH" version 2>/dev/null | head -1 || true)"
		[ -z "$current" ] || log "Found existing ${BIN_PATH} (${current}), checking for a newer release..."
	fi

	fetch_binary "$tmp_bin"
	chmod +x "$tmp_bin"
	mkdir -p "$INSTALL_DIR"
	mv "$tmp_bin" "$BIN_PATH"
	log "Installed ${BIN_PATH} (${VERSION})."

	case ":$PATH:" in
	*":${INSTALL_DIR}:"*) ;;
	*)
		warn "${INSTALL_DIR} is not on your PATH. Add this to your shell profile:"
		log "  export PATH=\"${INSTALL_DIR}:\$PATH\""
		;;
	esac

	log ""
	log "Next: point it at your running instance and log in."
	log "  export APP_API_URL=https://your-dashboard-domain"
	log "  ${BINARY_NAME} auth login --device"
}

main
