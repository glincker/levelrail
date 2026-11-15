#!/bin/sh
# Exercises install.sh's `protect` mode (the OOM protection drop-in) in a plain
# container with a fake systemctl, so no systemd or real install is needed.
#   scripts/test-install-protection.sh [base-image]
set -eu

base_image="${1:-ubuntu:24.04}"
repo_root="$(cd "$(dirname "$0")/.." && pwd)"
tag="install-protection-test:$(printf '%s' "$base_image" | tr ':/' '--')"
name="install-protection-test-$$"
drop=/etc/systemd/system/levelrail.service.d

cleanup() { docker rm -f "$name" >/dev/null 2>&1 || true; }
trap cleanup EXIT

docker build -q -t "$tag" - >/dev/null <<DOCKERFILE
FROM ${base_image}
ENV DEBIAN_FRONTEND=noninteractive
RUN apt-get update -qq && apt-get install -y -qq --no-install-recommends \
      curl ca-certificates iproute2 procps python3 >/dev/null \
 && rm -rf /var/lib/apt/lists/*
CMD ["sleep", "3600"]
DOCKERFILE

docker run -d --name "$name" "$tag" >/dev/null
docker cp "$repo_root/install.sh" "$name:/root/install.sh"

in_ct() { docker exec "$name" sh -c "$1"; }

in_ct "mkdir -p /run/systemd/system /fake $drop && : > /etc/systemd/system/levelrail.service"
in_ct 'printf "#!/bin/sh\nexit 0\n" > /fake/systemctl && chmod 755 /fake/systemctl'
in_ct "printf '[Service]\nEnvironment=APP_HTTP_ADDR=127.0.0.1:7000\n' > $drop/10-install.conf"
in_ct "printf '[Service]\nEnvironment=X=1\n' > $drop/trusted-proxies.conf"
in_ct "chmod 755 /root /root/install.sh"

fails=0
expect() {
	label="$1"
	out="$2"
	want="$3"
	if printf '%s' "$out" | grep -qF -- "$want"; then
		echo "ok   $label"
	else
		echo "FAIL $label (wanted: $want)"
		printf '%s\n' "$out" | sed 's/^/     | /'
		fails=$((fails + 1))
	fi
}
check() {
	label="$1"
	shift
	if in_ct "$*"; then echo "ok   $label"; else
		echo "FAIL $label"
		fails=$((fails + 1))
	fi
}

run() { docker exec "$name" env PATH="/fake:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin" "$@" sh /root/install.sh protect 2>&1 || true; }

out="$(docker exec -u nobody "$name" sh /root/install.sh protect --dry-run 2>&1)"
expect "dry run works without root" "$out" "OOMScoreAdjust=-900"
expect "dry run names the file" "$out" "30-protection.conf"
check "dry run wrote nothing" "test ! -e $drop/30-protection.conf"

out="$(docker exec -u nobody "$name" sh /root/install.sh protect 2>&1 || true)"
expect "non-root apply refused" "$out" "must run as root"

out="$(run)"
expect "apply writes the drop-in" "$out" "Wrote $drop/30-protection.conf"
expect "restart is advised" "$out" "systemctl restart levelrail"
out="$(run)"
expect "second run is a no-op" "$out" "already current"

check "10-install.conf untouched" "grep -q 127.0.0.1:7000 $drop/10-install.conf && ! grep -q OOMScore $drop/10-install.conf"
check "other drop-ins untouched" "grep -q X=1 $drop/trusted-proxies.conf"

in_ct "printf '[Service]\nOOMScoreAdjust=-500\n' > $drop/30-protection.conf"
out="$(run)"
expect "hand-edited file kept" "$out" "edited by hand"

in_ct "rm -f $drop/30-protection.conf"
out="$(run LEVELRAIL_PROTECT=0)"
expect "opt out" "$out" "LEVELRAIL_PROTECT=0"
out="$(run LEVELRAIL_OOM_SCORE_ADJUST=bad)"
expect "bad value refused" "$out" "must be integers"
out="$(run LEVELRAIL_OOM_SCORE_ADJUST=-700)"
expect "env override" "$out" "Wrote"
check "override value written" "grep -q OOMScoreAdjust=-700 $drop/30-protection.conf"

in_ct "rm -rf /run/systemd/system $drop/30-protection.conf"
out="$(run)"
expect "no systemd message" "$out" "systemd not detected"

[ "$fails" -eq 0 ] || {
	echo "$fails failure(s)"
	exit 1
}
echo "all protection checks passed"
