#!/usr/bin/env bash
# Build and sign the first-party plugins locally. Never pushes an image.
#
#   scripts/plugins-build.sh build [name...]   compile and docker build <registry>/levelrail-plugin-<name>:local
#   scripts/plugins-build.sh sign  [name...]   write dist/<name>/plugin.yaml (digest pinned) and plugin.sig
#   scripts/plugins-build.sh all   [name...]   build then sign
#
# Env: PLUGINS_OUT (default dist/plugins), PLUGINS_REGISTRY (default ghcr.io/glincker),
# PLUGINS_DIGEST_<NAME> or the registry digest file dist/<name>/digest to pin a pushed image,
# PLUGINS_KEY (publisher key file). With no PLUGINS_KEY a throwaway key is created for
# local tests only: never trust it outside a test control plane.
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
out="${PLUGINS_OUT:-$root/dist/plugins}"
registry="${PLUGINS_REGISTRY:-ghcr.io/glincker}"
goarch="${PLUGINS_GOARCH:-$(go env GOARCH)}"

all_plugins() {
	for d in "$root"/plugins/*/; do
		n="$(basename "$d")"
		[ "$n" = internal ] && continue
		[ -f "$d/plugin.yaml" ] && echo "$n"
	done
}

image_for() { echo "$registry/levelrail-plugin-$1"; }

build_one() {
	local n="$1" ctx="$out/$1"
	mkdir -p "$ctx"
	(cd "$root" && CGO_ENABLED=0 GOOS=linux GOARCH="$goarch" go build -trimpath -ldflags "-s -w" -o "$ctx/plugin" "./plugins/$n")
	cp "$root/plugins/$n/Dockerfile" "$ctx/Dockerfile"
	docker build -q -t "$(image_for "$n"):local" "$ctx" >/dev/null
	docker image inspect --format '{{.Id}}' "$(image_for "$n"):local" >"$ctx/image-id"
	echo "built $(image_for "$n"):local ($(cat "$ctx/image-id"))"
}

digest_for() {
	local n="$1" var
	var="PLUGINS_DIGEST_$(echo "$n" | tr 'a-z-' 'A-Z_')"
	if [ -n "${!var:-}" ]; then echo "${!var}"; return; fi
	if [ -f "$out/$n/digest" ]; then cat "$out/$n/digest"; return; fi
	cat "$out/$n/image-id"
}

sign_one() {
	local n="$1" ctx="$out/$1" key="${PLUGINS_KEY:-$out/test-publisher.key}" digest
	digest="$(digest_for "$n")"
	case "$digest" in sha256:????????????????????????????????????????????????????????????????) ;; *) echo "bad digest for $n: $digest" >&2; exit 1 ;; esac
	sed "s#^image: .*#image: $(image_for "$n")@$digest#" "$root/plugins/$n/plugin.yaml" >"$ctx/plugin.yaml"
	local res
	res="$(cd "$root" && go run ./test/fixtures/plugin-notify/signer -manifest "$ctx/plugin.yaml" -key "$key")"
	echo "$res" | sed -n 's/^signature: *//p' >"$ctx/plugin.sig"
	echo "$res" | sed -n 's/^public_key: *//p' >"$out/publisher.pub"
	echo "$res" | sed -n 's/^key_id: *//p' >"$out/publisher.key-id"
	echo "signed $n with key $(cat "$out/publisher.key-id")"
}

cmd="${1:-}"
[ -n "$cmd" ] || { sed -n '2,12p' "${BASH_SOURCE[0]}"; exit 2; }
shift || true
names=("$@")
if [ "${#names[@]}" -eq 0 ]; then
	while IFS= read -r n; do names+=("$n"); done < <(all_plugins)
fi
mkdir -p "$out"
for n in "${names[@]}"; do
	[ -f "$root/plugins/$n/plugin.yaml" ] || { echo "unknown plugin: $n" >&2; exit 2; }
	case "$cmd" in
	build) build_one "$n" ;;
	sign) sign_one "$n" ;;
	all) build_one "$n"; sign_one "$n" ;;
	*) echo "unknown command: $cmd" >&2; exit 2 ;;
	esac
done
if [ -z "${PLUGINS_KEY:-}" ] && { [ "$cmd" = sign ] || [ "$cmd" = all ]; }; then
	echo "NOTE: signed with the throwaway test key $out/test-publisher.key (tests only)."
fi
