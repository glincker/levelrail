#!/usr/bin/env bash
# Pulls the upstream images the live Docker tests use, with retries, before
# the test lane starts. A flaky or rate-limited registry then costs a retry
# here instead of a red nightly. Never fails: a test whose image is gone for
# good skips or fails on its own with a clear error.
# Usage: scripts/prepull-test-images.sh [image...]
set -uo pipefail

images=("$@")
if [ "${#images[@]}" -eq 0 ]; then
	images=(
		alpine:3.20 busybox:latest nginx:alpine redis:7 redis:7-alpine postgres:16
		mysql:8 mariadb:11 mongo:7 clickhouse/clickhouse-server:24.8
		traefik/whoami:v1.10
	)
fi

pull() {
	local img="$1" n
	for n in 1 2 3; do
		docker pull -q "$img" >/dev/null 2>&1 && return 0
		sleep $((n * 5))
	done
	echo "::warning::could not pull $img after 3 tries"
}

for img in "${images[@]}"; do
	pull "$img" &
	# bounded fan-out: registries rate-limit bursts
	while [ "$(jobs -rp | wc -l)" -ge 4 ]; do sleep 1; done
done
wait
exit 0
