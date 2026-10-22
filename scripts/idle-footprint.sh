#!/usr/bin/env bash
# Idle footprint of the control plane inside a clean Linux container, as JSON.
#
# Usage:
#   scripts/idle-footprint.sh                                  # 0, 5 and 20 stopped apps
#   IDLE_COUNTS=0,5 IDLE_WINDOW_SECONDS=30 scripts/idle-footprint.sh
#   IDLE_APP_MODE=running scripts/idle-footprint.sh            # real containers via the host Docker socket
#   IDLE_FOOTPRINT_RECORD=1 scripts/idle-footprint.sh          # merge the run into the baseline
#   IDLE_FOOTPRINT_CHECK=1 IDLE_TOL_RSS_PCT=15 IDLE_TOL_GOROUTINES=10 \
#     IDLE_TOL_FDS=10 IDLE_TOL_CPU_POINTS=0.3 scripts/idle-footprint.sh   # fail on regression
#
# The check is off unless IDLE_FOOTPRINT_CHECK=1 and has no default tolerances.
# On Docker Desktop the container runs in a Linux VM, so the numbers are not bare metal.

set -euo pipefail
repo_root="$(git rev-parse --show-toplevel)"
cd "$repo_root"

: "${IDLE_IMAGE:=debian:bookworm-slim}"
: "${IDLE_COUNTS:=0,5,20}"
: "${IDLE_APP_MODE:=stopped}"
: "${IDLE_WINDOW_SECONDS:=60}"
: "${IDLE_SETTLE_SECONDS:=20}"
: "${IDLE_BASELINE:=scripts/idle-footprint-baseline.json}"

host_arch="$(docker version --format '{{.Server.Arch}}')"
case "$host_arch" in
	amd64 | arm64) ;;
	aarch64) host_arch=arm64 ;;
	x86_64) host_arch=amd64 ;;
	*) echo "unsupported docker server arch: $host_arch" >&2; exit 2 ;;
esac
arch="${IDLE_ARCH:-$host_arch}"

work="$(mktemp -d -t idle-footprint.XXXXXX)"
trap 'rm -rf "$work"' EXIT
out="${IDLE_OUT:-$work/result.json}"

echo "idle-footprint: building linux/$arch binaries" >&2
GOOS=linux GOARCH="$arch" CGO_ENABLED=0 go build -trimpath -o "$work/control-plane" ./cmd/levelrail
GOOS=linux GOARCH="$arch" CGO_ENABLED=0 go build -trimpath -o "$work/idle-footprint" ./scripts/idle-footprint
cp brand.yaml "$work/brand.yaml"
cp test/fixtures/dev-fixtures.yml "$work/dev-fixtures.yml"

docker_args=(--rm --platform "linux/$arch" -v "$work:/idle:ro" -w /idle
	-e IDLE_BIN=/idle/control-plane -e IDLE_BRAND_FILE=/idle/brand.yaml -e IDLE_FIXTURES_FILE=/idle/dev-fixtures.yml
	-e IDLE_LOOPS_DUMP -e IDLE_COUNTS -e IDLE_APP_MODE -e IDLE_WINDOW_SECONDS -e IDLE_SETTLE_SECONDS)
[ -n "${IDLE_APP_IMAGE:-}" ] && docker_args+=(-e IDLE_APP_IMAGE)
if [ "$IDLE_APP_MODE" = "running" ]; then
	sock_gid="$(docker run --rm -v /var/run/docker.sock:/var/run/docker.sock "$IDLE_IMAGE" stat -c '%g' /var/run/docker.sock)"
	docker_args+=(-v /var/run/docker.sock:/var/run/docker.sock --group-add "$sock_gid" --user 0)
fi

echo "idle-footprint: running in $IDLE_IMAGE (counts $IDLE_COUNTS, mode $IDLE_APP_MODE, window ${IDLE_WINDOW_SECONDS}s)" >&2
docker info --format 'idle-footprint: docker server {{.ServerVersion}}, {{.OperatingSystem}}, {{.NCPU}} CPUs, {{.MemTotal}} bytes RAM' >&2 || true
docker run "${docker_args[@]}" "$IDLE_IMAGE" /idle/idle-footprint measure >"$out"
cat "$out"

if [ -n "${IDLE_FOOTPRINT_RECORD:-}" ]; then
	go run ./scripts/idle-footprint record "$IDLE_BASELINE" "$out"
	echo "idle-footprint: recorded into $IDLE_BASELINE" >&2
fi
if [ "${IDLE_FOOTPRINT_CHECK:-}" = "1" ]; then
	go run ./scripts/idle-footprint check "$IDLE_BASELINE" "$out"
	echo "idle-footprint: within tolerance of $IDLE_BASELINE" >&2
fi
