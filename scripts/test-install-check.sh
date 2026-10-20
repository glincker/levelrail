#!/bin/sh
# Exercises `install.sh --check` (the read-only server readiness report) in a
# plain container: every port and proxy situation is a fixture, nothing is
# installed. Much faster than scripts/test-install-sh.sh, which needs systemd.
#   scripts/test-install-check.sh [base-image]
set -eu

base_image="${1:-ubuntu:24.04}"
repo_root="$(cd "$(dirname "$0")/.." && pwd)"
tag="install-check-test:$(printf '%s' "$base_image" | tr ':/' '--')"
name="install-check-test-$$"

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

# The report needs a systemd-shaped host to pass its own OS rows.
in_ct 'mkdir -p /run/systemd/system /fake && printf "#!/bin/sh\nexit 3\n" > /fake/systemctl && chmod 755 /fake/systemctl'

check() {
	docker exec "$name" env PATH="/fake:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin" \
		LEVELRAIL_MIN_DISK_GB=1 "$@" sh /root/install.sh --check 2>&1
}

expect() {
	label="$1"
	out="$2"
	want="$3"
	printf '%s' "$out" | grep -qF -- "$want" || {
		echo "FAIL ${label}: missing '${want}' in:"
		printf '%s\n' "$out"
		exit 1
	}
}

refute() {
	label="$1"
	out="$2"
	unwanted="$3"
	printf '%s' "$out" | grep -qF -- "$unwanted" && {
		echo "FAIL ${label}: unexpected '${unwanted}' in:"
		printf '%s\n' "$out"
		exit 1
	}
	return 0
}

echo "== free ports: own ports mode, exit 0"
rc=0
out="$(check)" || rc=$?
[ "$rc" -eq 0 ] || { echo "FAIL free ports: exit $rc"; printf '%s\n' "$out"; exit 1; }
expect "free ports" "$out" "Recommended mode: own ports"
expect "free ports" "$out" "install.sh | sudo sh"
refute "free ports" "$out" "--coexist"

echo "== --check never writes anything"
in_ct 'test ! -e /etc/systemd/system/levelrail.service && test ! -e /usr/local/bin/levelrail && test ! -d /var/lib/levelrail-data' ||
	{ echo "FAIL: --check changed the host"; exit 1; }

echo "== port 80 held by an unknown process: behind an existing proxy"
in_ct 'nohup python3 -m http.server 80 --bind 0.0.0.0 >/dev/null 2>&1 & echo $! > /tmp/p80.pid; sleep 1'
rc=0
out="$(check)" || rc=$?
[ "$rc" -eq 0 ] || { echo "FAIL held 80: exit $rc"; printf '%s\n' "$out"; exit 1; }
expect "held 80" "$out" "Recommended mode: behind your existing proxy"
expect "held 80" "$out" "python3"
expect "held 80" "$out" "sh -s -- --coexist"

echo "== a Traefik container publishing the port is named"
# shellcheck disable=SC2016 # runs inside the container, expands there
in_ct 'cat > /fake/docker <<"SH"
#!/bin/sh
case "$1" in
version) echo 27.0.1 ;;
ps) echo "container coolify-proxy (image traefik:v3.1)" ;;
info) echo "[name=seccomp,profile=builtin]" ;;
esac
SH
chmod 755 /fake/docker'
out="$(check)" || true
expect "traefik" "$out" "container coolify-proxy (image traefik:v3.1)"
expect "traefik" "$out" "Recommended mode: behind your existing proxy"

echo "== --domain adds the exact proxy verification step"
out="$(check LEVELRAIL_DOMAIN=console.example.test)" || true
expect "domain" "$out" "--coexist --domain console.example.test"
expect "domain" "$out" "levelrail-cli proxy --domain console.example.test --verify"
expect "domain" "$out" "console.example.test does not resolve"

echo "== rootless Docker is flagged"
# shellcheck disable=SC2016 # runs inside the container, expands there
in_ct 'cat > /fake/docker <<"SH"
#!/bin/sh
case "$1" in
version) echo 27.0.1 ;;
ps) : ;;
info) echo "[name=seccomp,profile=builtin name=rootless]" ;;
esac
SH
chmod 755 /fake/docker'
in_ct 'kill "$(cat /tmp/p80.pid)"' || true
sleep 1
out="$(check)" || true
expect "rootless" "$out" "rootless Docker cannot publish ports 80/443"
expect "rootless" "$out" "Recommended mode: own ports"

echo "== an exposed Docker API (2375 on all interfaces) fails the check"
in_ct 'nohup python3 -m http.server 2375 --bind 0.0.0.0 >/dev/null 2>&1 & echo $! > /tmp/p2375.pid; sleep 1'
rc=0
out="$(check)" || rc=$?
[ "$rc" -ne 0 ] || { echo "FAIL exposed API: exit 0"; printf '%s\n' "$out"; exit 1; }
expect "exposed API" "$out" "the Docker API listens on a network address without TLS"
in_ct 'kill "$(cat /tmp/p2375.pid)"' || true

echo "== install with --no-coexist keeps the failing preflight"
in_ct 'nohup python3 -m http.server 80 --bind 0.0.0.0 >/dev/null 2>&1 & echo $! > /tmp/p80.pid; sleep 1'
rc=0
out="$(docker exec "$name" env PATH="/fake:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin" \
	LEVELRAIL_MIN_DISK_GB=1 LEVELRAIL_BINARY_FILE=/nonexistent sh /root/install.sh --no-coexist 2>&1)" || rc=$?
[ "$rc" -ne 0 ] || { echo "FAIL: --no-coexist install should fail preflight"; exit 1; }
expect "no-coexist" "$out" "Not switching to coexist mode"
expect "no-coexist" "$out" "LEVELRAIL_HTTP_PORT/LEVELRAIL_HTTPS_PORT"

echo "install.sh --check passed on ${base_image}"
