#!/usr/bin/env bash
# Prints the Go packages in one CI test group, one import path per line.
#
#   api     internal/api alone (the slowest package, sharded by test name)
#   docker  packages whose tests talk to a real Docker daemon
#   rest    everything else
#
# The docker group runs in its own job with bounded -p so heavy live tests
# (Postgres, MinIO, BuildKit) can't OOM the runner by starting together.
#
# Usage: scripts/go-test-groups.sh <api|docker|rest>
set -euo pipefail

group="${1:?usage: go-test-groups.sh <api|docker|rest>}"
cd "$(git rev-parse --show-toplevel)"

api_pkg="github.com/GLINCKER/levelrail/internal/api"

uses_docker() {
	local dir="$1"
	case "$dir" in
	*/test/*) return 0 ;;
	esac
	compgen -G "$dir/*_live_test.go" >/dev/null && return 0
	compgen -G "$dir/*_test.go" >/dev/null || return 1
	grep -qE 'internal/dockertest"|docker\.NewClient\(|docker/docker/client"|moby/moby/client"' "$dir"/*_test.go
}

go list -f '{{.ImportPath}} {{.Dir}}' ./... | while read -r pkg dir; do
	if [ "$pkg" = "$api_pkg" ]; then
		if [ "$group" = "api" ]; then echo "$pkg"; fi
		continue
	fi
	case "$group" in
	api) ;;
	docker) if uses_docker "$dir"; then echo "$pkg"; fi ;;
	rest) if ! uses_docker "$dir"; then echo "$pkg"; fi ;;
	*)
		echo "unknown group: $group" >&2
		exit 2
		;;
	esac
done
