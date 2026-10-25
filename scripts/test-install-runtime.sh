#!/bin/sh
# Exercises install.sh --runtime (Docker or Podman) in a plain container with
# stub docker, podman, systemctl and curl binaries on PATH. Nothing real is
# installed and no get.docker.com request leaves the container.
#   scripts/test-install-runtime.sh [base-image]
set -eu

base_image="${1:-ubuntu:24.04}"
repo_root="$(cd "$(dirname "$0")/.." && pwd)"
tag="install-runtime-test:$(printf '%s' "$base_image" | tr ':/' '--')"
name="install-runtime-test-$$"

cleanup() { docker rm -f "$name" >/dev/null 2>&1 || true; }
trap cleanup EXIT

docker build -q -t "$tag" - >/dev/null <<EOF
FROM ${base_image}
ENV DEBIAN_FRONTEND=noninteractive
RUN apt-get update -qq && apt-get install -y -qq --no-install-recommends \
      curl ca-certificates iproute2 procps >/dev/null \
 && rm -rf /var/lib/apt/lists/*
CMD ["sleep", "3600"]
EOF

docker run -d --name "$name" "$tag" >/dev/null
docker cp "$repo_root/install.sh" "$name:/root/install.sh"

in_ct() { docker exec "$name" sh -c "$1"; }

# systemctl and curl log every call; is-active reports inactive like a fresh host.
# shellcheck disable=SC2016 # runs inside the container, expands there
in_ct 'mkdir -p /run/systemd/system /fake /stubs
cat > /fake/systemctl <<"SH"
#!/bin/sh
echo "systemctl $*" >> /tmp/calls.log
case "$1" in is-active) exit 3 ;; esac
exit 0
SH
cat > /fake/curl <<"SH"
#!/bin/sh
echo "curl $*" >> /tmp/calls.log
exit 0
SH
printf "#!/bin/sh\necho stub-binary\n" > /stubs/levelrail
chmod 755 /fake/systemctl /fake/curl /stubs/levelrail'

stub_docker() {
	# shellcheck disable=SC2016 # runs inside the container, expands there
	in_ct 'cat > /fake/docker <<"SH"
#!/bin/sh
case "$1" in
--version) echo "Docker version 27.0.1, build abc" ;;
version) echo 27.0.1 ;;
ps) : ;;
info) echo "[name=seccomp,profile=builtin]" ;;
esac
SH
chmod 755 /fake/docker'
}

stub_podman() {
	# shellcheck disable=SC2016 # runs inside the container, expands there
	in_ct 'cat > /fake/podman <<"SH"
#!/bin/sh
case "$1" in
--version) echo "podman version 5.2.2" ;;
info) case "$*" in *Rootless*) echo false ;; esac ;;
esac
SH
chmod 755 /fake/podman'
}

reset() {
	in_ct 'rm -f /fake/docker /fake/podman /tmp/calls.log /etc/systemd/system/levelrail.service; rm -rf /etc/systemd/system/levelrail.service.d /var/lib/levelrail-data /usr/local/bin/levelrail'
}

run() {
	docker exec "$name" env PATH="/fake:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin" \
		LEVELRAIL_MIN_DISK_GB=1 LEVELRAIL_HEALTH_WAIT=1 LEVELRAIL_BINARY_FILE=/stubs/levelrail \
		LEVELRAIL_PUBLIC_IP=127.0.0.1 LEVELRAIL_SKIP_REACHABILITY=1 LEVELRAIL_INSTALL_CLI=0 \
		"$@" 2>&1
}

expect() {
	printf '%s' "$2" | grep -qF -- "$3" || { echo "FAIL $1: missing '$3' in:"; printf '%s\n' "$2"; exit 1; }
}
refute() {
	if printf '%s' "$2" | grep -qF -- "$3"; then echo "FAIL $1: unexpected '$3' in:"; printf '%s\n' "$2"; exit 1; fi
}
calls() { in_ct 'cat /tmp/calls.log 2>/dev/null || true'; }

echo "== podman missing: non-zero, distro hint, nothing installed"
reset
rc=0
out="$(run sh /root/install.sh --runtime podman)" || rc=$?
[ "$rc" -ne 0 ] || { echo "FAIL: missing podman should fail"; exit 1; }
expect "podman missing" "$out" "apt install podman"
expect "podman missing" "$out" "dnf install podman"
refute "podman missing" "$(calls)" "get.docker.com"

echo "== podman missing also fails --check"
rc=0
out="$(run sh /root/install.sh --check --runtime podman)" || rc=$?
[ "$rc" -ne 0 ] || { echo "FAIL: --check with missing podman should fail"; exit 1; }
expect "check podman missing" "$out" "apt install podman"

echo "== --runtime podman, no docker: socket enabled, env var written, no get.docker.com"
reset
stub_podman
out="$(run sh /root/install.sh --runtime podman)" || true
expect "podman" "$out" "Podman 5.2.2 (rootful): best-effort"
expect "podman" "$out" "Dockerfile and Railpack source builds are not verified on Podman yet."
expect "podman" "$(calls)" "systemctl enable --now podman.socket"
refute "podman" "$(calls)" "get.docker.com"
refute "podman" "$(calls)" "enable --now docker"
in_ct 'grep -qx "Environment=APP_CONTAINER_RUNTIME_SOCKET=unix:///run/podman/podman.sock" /etc/systemd/system/levelrail.service.d/10-install.conf' ||
	{ echo "FAIL: drop-in lacks APP_CONTAINER_RUNTIME_SOCKET"; exit 1; }
in_ct 'grep -qx "Requires=podman.socket" /etc/systemd/system/levelrail.service && ! grep -q "docker.service" /etc/systemd/system/levelrail.service' ||
	{ echo "FAIL: unit should order after podman.socket, not docker.service"; exit 1; }

echo "== auto with docker present: unchanged Docker path"
reset
stub_docker
out="$(run sh /root/install.sh)" || true
expect "auto docker" "$out" "Docker 27 (rootful): supported"
refute "auto docker" "$out" "Podman"
expect "auto docker" "$(calls)" "systemctl enable --now docker"
refute "auto docker" "$(calls)" "get.docker.com"
in_ct '! grep -q APP_CONTAINER_RUNTIME_SOCKET /etc/systemd/system/levelrail.service.d/10-install.conf && grep -qx "Requires=docker.service" /etc/systemd/system/levelrail.service' ||
	{ echo "FAIL: docker install should keep docker.service and no runtime socket"; exit 1; }

echo "== auto with only podman: Podman is used"
reset
stub_podman
out="$(run sh /root/install.sh)" || true
expect "auto podman" "$out" "Podman 5.2.2 (rootful)"
expect "auto podman" "$(calls)" "systemctl enable --now podman.socket"
refute "auto podman" "$(calls)" "get.docker.com"

echo "== auto with neither: Docker is installed as before"
reset
out="$(run sh /root/install.sh)" || true
expect "auto none" "$out" "Docker will be installed via get.docker.com: supported"
expect "auto none" "$(calls)" "get.docker.com"

echo "== --runtime existing with neither: fails, installs nothing"
reset
rc=0
out="$(run sh /root/install.sh --runtime existing)" || rc=$?
[ "$rc" -ne 0 ] || { echo "FAIL: existing with no runtime should fail"; exit 1; }
refute "existing" "$(calls)" "get.docker.com"

echo "== --check shows Podman rows and exits 0"
reset
stub_podman
rc=0
out="$(run sh /root/install.sh --check)" || rc=$?
[ "$rc" -eq 0 ] || { echo "FAIL: --check with podman exit $rc"; printf '%s\n' "$out"; exit 1; }
expect "check podman" "$out" "Podman 5.2.2 (rootful): best-effort"

echo "== bad --runtime value is rejected"
rc=0
out="$(run sh /root/install.sh --runtime containerd)" || rc=$?
[ "$rc" -ne 0 ] || { echo "FAIL: bad runtime accepted"; exit 1; }

echo "install.sh --runtime passed on ${base_image}"
