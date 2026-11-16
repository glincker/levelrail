#!/bin/sh
# Checks install.sh's fresh-install Docker guard default without a container:
# the two helper functions are extracted and run against temp dirs.
#   scripts/test-install-docker-guard.sh
set -eu

repo_root="$(cd "$(dirname "$0")/.." && pwd)"
work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT

# Pull is_fresh_install and seed_docker_guard out of install.sh as written.
awk '/^is_fresh_install\(\) \{/,/^}/ {print} /^seed_docker_guard\(\) \{/,/^}/ {print}' \
	"$repo_root/install.sh" >"$work/funcs.sh"
grep -q seed_docker_guard "$work/funcs.sh" || { echo "FAIL: seed_docker_guard not found"; exit 1; }
grep -Fq '"$FRESH_INSTALL" -eq 0 ] || seed_docker_guard' "$repo_root/install.sh" ||
	{ echo "FAIL: do_install no longer gates the seed on a fresh install"; exit 1; }

# scenario <name> <runtime> <unit:0|1> <guard:0|1> <other-file:0|1>; prints the data dir.
scenario() {
	d="$work/$1"
	mkdir -p "$d/data" "$d/etc"
	[ "$3" -eq 0 ] || : >"$d/etc/levelrail.service"
	[ "$4" -eq 0 ] || printf '{"mode":"audit"}\n' >"$d/data/docker-guard.json"
	[ "$5" -eq 0 ] || : >"$d/data/levelrail.db"
	(
		DATA_DIR="$d/data"
		UNIT_PATH="$d/etc/levelrail.service"
		RUNTIME="$2"
		FRESH_INSTALL=0
		# shellcheck disable=SC1091
		. "$work/funcs.sh"
		if is_fresh_install; then FRESH_INSTALL=1; fi
		[ "$FRESH_INSTALL" -eq 0 ] || seed_docker_guard
	)
	printf '%s' "$d/data"
}

mode_of() { stat -c '%a' "$1" 2>/dev/null || stat -f '%Lp' "$1"; }

echo "== fresh install writes enforce with mode 0600"
data="$(scenario fresh docker 0 0 0)"
grep -q '"mode": "enforce"' "$data/docker-guard.json" || { echo "FAIL fresh: not enforce"; exit 1; }
[ "$(mode_of "$data/docker-guard.json")" = 600 ] || { echo "FAIL fresh: mode is not 0600"; exit 1; }

echo "== an existing guard file is untouched"
data="$(scenario guard docker 0 1 0)"
grep -q '"mode":"audit"' "$data/docker-guard.json" || { echo "FAIL guard: file was rewritten"; exit 1; }

echo "== an existing unit file (upgrade) writes nothing"
data="$(scenario upgrade docker 1 0 0)"
[ ! -e "$data/docker-guard.json" ] || { echo "FAIL upgrade: guard file written"; exit 1; }

echo "== a populated data dir writes nothing"
data="$(scenario populated docker 0 0 1)"
[ ! -e "$data/docker-guard.json" ] || { echo "FAIL populated: guard file written"; exit 1; }

echo "== a Podman host writes nothing"
data="$(scenario podman podman 0 0 0)"
[ ! -e "$data/docker-guard.json" ] || { echo "FAIL podman: guard file written"; exit 1; }

echo "ok"
