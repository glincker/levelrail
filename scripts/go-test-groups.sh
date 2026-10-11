#!/usr/bin/env bash
# Prints the Go packages in one CI test group, one import path per line.
#
#   api     internal/api alone (the slowest package, sharded by test name)
#   docker  packages whose tests talk to a real Docker daemon
#   rest    everything else
#   rest-heavy / rest-light   rest split by scripts/ci-heavy-packages.txt (nightly)
#   race    non-api, non-docker packages that start goroutines or use sync/atomic/chan
#
# The docker group runs in its own job with bounded -p so heavy live tests
# (Postgres, MinIO, BuildKit) can't OOM the runner by starting together.
#
# Usage: scripts/go-test-groups.sh <api|docker|rest|rest-heavy|rest-light|race> [package...]
# With packages, classifies only those instead of ./...
set -euo pipefail

group="${1:?usage: go-test-groups.sh <api|docker|rest|rest-heavy|rest-light|race> [package...]}"
shift
[ "$#" -gt 0 ] || set -- ./...
cd "$(git rev-parse --show-toplevel)"

api_pkg="github.com/GLINCKER/levelrail/internal/api"

uses_docker() {
	local dir="$1"
	case "$dir" in
	*/test/execgate) return 1 ;;
	*/test/*) return 0 ;;
	esac
	compgen -G "$dir/*_live_test.go" >/dev/null && return 0
	compgen -G "$dir/*_test.go" >/dev/null || return 1
	grep -qE 'internal/dockertest"|docker\.NewClient\(|docker/docker/client"|moby/moby/client"' "$dir"/*_test.go
}

heavy_file=scripts/ci-heavy-packages.txt
is_heavy() {
	[ -f "$heavy_file" ] || return 1
	grep -vE '^(#|$)' "$heavy_file" | grep -qxF "${1#github.com/GLINCKER/levelrail/}"
}

is_concurrent() {
	local f files=()
	for f in "$1"/*.go; do
		[ -f "$f" ] || continue
		case "$f" in *_test.go) continue ;; esac
		files+=("$f")
	done
	[ "${#files[@]}" -gt 0 ] || return 1
	grep -qE '\bgo (func|[A-Za-z_.]+\()|sync\.|atomic\.|\bchan\b' "${files[@]}"
}

go list -e -f '{{.ImportPath}} {{.Dir}}' "$@" | while read -r pkg dir; do
	if [ "$pkg" = "$api_pkg" ]; then
		if [ "$group" = "api" ]; then echo "$pkg"; fi
		continue
	fi
	case "$group" in
	api) ;;
	docker) if uses_docker "$dir"; then echo "$pkg"; fi ;;
	rest) if ! uses_docker "$dir"; then echo "$pkg"; fi ;;
	rest-heavy) if ! uses_docker "$dir" && is_heavy "$pkg"; then echo "$pkg"; fi ;;
	rest-light) if ! uses_docker "$dir" && ! is_heavy "$pkg"; then echo "$pkg"; fi ;;
	race) if ! uses_docker "$dir" && is_concurrent "$dir"; then echo "$pkg"; fi ;;
	*)
		echo "unknown group: $group" >&2
		exit 2
		;;
	esac
done
