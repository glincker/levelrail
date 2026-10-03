#!/usr/bin/env bash
# Runs go test through gotestsum and reports the slowest individual
# tests, so the --jsonfile + `tool slowest` two-step doesn't have to be
# hand-assembled every time a package feels slow.
#
# Usage: scripts/go-test-slowest.sh [-t <threshold>] <package>...
#
# Environment:
#   GO_TEST_FLAGS  extra go test flags (e.g. "-short")
set -uo pipefail

threshold="500ms"
while getopts "t:" opt; do
	case "$opt" in
	t) threshold="$OPTARG" ;;
	*)
		echo "usage: go-test-slowest.sh [-t <threshold>] <package>..." >&2
		exit 2
		;;
	esac
done
shift $((OPTIND - 1))

[ "$#" -gt 0 ] || {
	echo "go-test-slowest.sh: no packages given" >&2
	exit 2
}

command -v gotestsum >/dev/null 2>&1 || {
	echo "go-test-slowest.sh: gotestsum not found, run: go -C tools install gotest.tools/gotestsum" >&2
	exit 2
}

root="$(git rev-parse --show-toplevel)"
cd "$root" || exit 2

out="$(mktemp -d)"
trap 'rm -rf "$out"' EXIT

read -r -a flags <<<"${GO_TEST_FLAGS:-}"

gotestsum --jsonfile "$out/events.json" --format pkgname --packages "$*" -- "${flags[@]}"
status=$?

echo
gotestsum tool slowest --jsonfile "$out/events.json" --threshold "$threshold"

exit "$status"
