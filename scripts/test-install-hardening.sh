#!/bin/sh
# Exercises install.sh's refusal, uninstall and re-run branches in a plain
# container with a fake systemctl and fake "binaries" (shell scripts that answer
# `version --json`), so no systemd, Docker or real release is needed.
#   scripts/test-install-hardening.sh [base-image]
set -eu

base_image="${1:-ubuntu:24.04}"
repo_root="$(cd "$(dirname "$0")/.." && pwd)"
tag="install-hardening-test:$(printf '%s' "$base_image" | tr ':/' '--')"
name="install-hardening-test-$$"

cleanup() { docker rm -f "$name" >/dev/null 2>&1 || true; }
trap cleanup EXIT

docker build -q -t "$tag" - >/dev/null <<EOF
FROM ${base_image}
ENV DEBIAN_FRONTEND=noninteractive
RUN apt-get update -qq && apt-get install -y -qq --no-install-recommends \
      curl ca-certificates iproute2 procps python3 >/dev/null \
 && rm -rf /var/lib/apt/lists/*
CMD ["sleep", "3600"]
EOF

docker run -d --name "$name" "$tag" >/dev/null
docker cp "$repo_root/install.sh" "$name:/root/install.sh"

in_ct() { docker exec "$name" sh -c "$1"; }

in_ct 'mkdir -p /run/systemd/system /fake /opt/mirror'
# shellcheck disable=SC2016 # runs inside the container, expands there
in_ct 'cat > /fake/systemctl <<"SH"
#!/bin/sh
case "$1" in
is-active) [ -f /tmp/svc-active ] ;;
*) exit 0 ;;
esac
SH
chmod 755 /fake/systemctl'
in_ct 'printf "#!/bin/sh\nexit 0\n" > /fake/docker && chmod 755 /fake/docker'

# fake_bin <path> <version> <schema>: a script the installer treats as a release binary.
fake_bin() {
	# shellcheck disable=SC2016 # runs inside the container, expands there
	in_ct "cat > $1 <<'SH'
#!/bin/sh
# usage: levelrail version [--json]
# retain --version <tag> --file <binary>
case \"\$1\" in
version)
	if [ \"\${2:-}\" = \"--json\" ]; then echo '{\"version\":\"$2\",\"schema_version\":$3,\"os\":\"linux\",\"arch\":\"x\"}'; else echo '$2 (schema $3, linux/x)'; fi ;;
retain) exit 0 ;;
esac
SH
chmod 755 $1"
}

run() {
	docker exec "$name" env PATH="/fake:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin" \
		LEVELRAIL_MIN_DISK_GB=1 LEVELRAIL_NO_COSIGN=1 "$@" 2>&1
}

expect() {
	printf '%s' "$2" | grep -qF -- "$3" || {
		echo "FAIL $1: missing '$3' in:"
		printf '%s\n' "$2"
		exit 1
	}
}

refute() {
	if printf '%s' "$2" | grep -qF -- "$3"; then
		echo "FAIL $1: unexpected '$3' in:"
		printf '%s\n' "$2"
		exit 1
	fi
}

echo "== uninstall on a never-installed host says so"
out="$(run sh /root/install.sh uninstall)"
expect "never installed" "$out" "Nothing to remove"
refute "never installed" "$out" "Removed the"

echo "== upgrade to an older schema is refused before anything changes"
fake_bin /usr/local/bin/levelrail v0.6.0 1010
fake_bin /root/old v0.5.1 795
before="$(in_ct 'sha256sum /usr/local/bin/levelrail')"
rc=0
out="$(run env LEVELRAIL_BINARY_FILE=/root/old LEVELRAIL_VERSION=v0.5.1 sh /root/install.sh upgrade)" || rc=$?
[ "$rc" -ne 0 ] || { echo "FAIL downgrade: exit 0"; exit 1; }
expect "downgrade" "$out" "database is at schema 1010 and v0.5.1 supports schema 795"
expect "downgrade" "$out" "Safe next step"
refute "downgrade" "$out" "did not become healthy"
[ "$before" = "$(in_ct 'sha256sum /usr/local/bin/levelrail')" ] || { echo "FAIL downgrade: binary was replaced"; exit 1; }
in_ct 'test ! -e /usr/local/bin/levelrail.pre-upgrade' || { echo "FAIL downgrade: stray .pre-upgrade left"; exit 1; }

echo "== a newer schema is accepted"
fake_bin /root/new v0.7.0 1100
in_ct 'touch /tmp/svc-active'
out="$(run env LEVELRAIL_BINARY_FILE=/root/new LEVELRAIL_VERSION=v0.7.0 LEVELRAIL_HEALTH_WAIT=2 sh /root/install.sh upgrade)" || true
refute "upgrade" "$out" "cannot move to"
in_ct 'rm -f /tmp/svc-active'

echo "== retain does not suggest a rollback the schema blocks"
fake_bin /usr/local/bin/levelrail v0.6.0 1010
out="$(run env LEVELRAIL_BINARY_FILE=/root/old LEVELRAIL_VERSION=v0.5.1 sh /root/install.sh retain)"
expect "retain blocked" "$out" "blocked by the database schema"
refute "retain blocked" "$out" "Next, on this host"
out="$(run env LEVELRAIL_BINARY_FILE=/root/new LEVELRAIL_VERSION=v0.7.0 sh /root/install.sh retain)"
expect "retain ok" "$out" "Next, on this host: sudo levelrail rollback --to v0.7.0"

echo "== check on an installed host says so, not that 80/443 are free"
in_ct 'touch /tmp/svc-active'
out="$(run sh /root/install.sh check)"
expect "installed check" "$out" "already installed and running (v0.6.0)"
refute "installed check" "$out" "are free"
refute "installed check" "$out" "Recommended mode"
in_ct 'rm -f /tmp/svc-active'

echo "== a failed preflight drops the recommendation and names supported hosts"
in_ct 'rm -rf /run/systemd/system'
rc=0
out="$(run sh /root/install.sh check)" || rc=$?
[ "$rc" -ne 0 ] || { echo "FAIL failed check: exit 0"; exit 1; }
expect "failed check" "$out" "Supported hosts"
refute "failed check" "$out" "Recommended mode"
refute "failed check" "$out" "Next step"
in_ct 'mkdir -p /run/systemd/system'

echo "== uninstall removes retained releases and the CLI; purge removes cosign only if installed here"
in_ct 'mkdir -p /usr/local/bin/levelrail.releases /var/lib/levelrail-data && touch /usr/local/bin/levelrail.releases/x /usr/local/bin/levelrail-cli /usr/local/bin/cosign'
out="$(run sh /root/install.sh uninstall --purge)"
expect "uninstall" "$out" "retained releases"
in_ct 'test ! -e /usr/local/bin/levelrail.releases && test ! -e /usr/local/bin/levelrail-cli && test ! -e /usr/local/bin/levelrail' ||
	{ echo "FAIL uninstall: leftovers"; exit 1; }
in_ct 'test -e /usr/local/bin/cosign' || { echo "FAIL uninstall: removed a cosign it did not install"; exit 1; }
fake_bin /usr/local/bin/levelrail v0.6.0 1010
in_ct 'touch /usr/local/bin/cosign.installed-by-levelrail'
run sh /root/install.sh uninstall --purge >/dev/null
in_ct 'test ! -e /usr/local/bin/cosign && test ! -e /usr/local/bin/cosign.installed-by-levelrail' ||
	{ echo "FAIL purge: cosign installed here was kept"; exit 1; }

echo "== re-run skips the download when version and checksum match, and same-version upgrade is a no-op"
fake_bin /usr/local/bin/levelrail v0.6.0 1010
# shellcheck disable=SC2016 # runs inside the container, expands there
in_ct 'arch="$(uname -m)"; case "$arch" in x86_64) a=amd64 ;; *) a=arm64 ;; esac
cp /usr/local/bin/levelrail "/opt/mirror/levelrail-linux-$a"
(cd /opt/mirror && sha256sum "levelrail-linux-$a" > checksums.txt)
mkdir -p /opt/mirror/dl && cd /opt/mirror && nohup python3 -m http.server 8099 --bind 127.0.0.1 >/dev/null 2>&1 &
sleep 1'
in_ct 'touch /tmp/svc-active'
out="$(run env LEVELRAIL_RELEASE_BASE_URL=http://127.0.0.1:8099 LEVELRAIL_INSECURE_MIRROR=1 LEVELRAIL_VERSION=v0.6.0 sh /root/install.sh upgrade)"
expect "skip download" "$out" "skipping the download"
expect "skip download" "$out" "Already on v0.6.0, nothing to do"
refute "skip download" "$out" "Downloading"

echo "== curl is required even with --force"
in_ct 'mv /usr/bin/curl /usr/bin/curl.off'
rc=0
out="$(run sh /root/install.sh --force)" || rc=$?
[ "$rc" -ne 0 ] || { echo "FAIL no curl: exit 0"; exit 1; }
expect "no curl" "$out" "curl is required and not installed"
refute "no curl" "$out" "Installing Docker"

echo "install.sh hardening passed on ${base_image}"
