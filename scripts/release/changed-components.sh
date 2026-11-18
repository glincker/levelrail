#!/usr/bin/env bash
# Prints true when a release component's inputs changed between two refs, else false.
# Usage: scripts/release/changed-components.sh <component> <base-ref> [head-ref]
# Components: mcp, cli, agent, terraform. Run inside a checkout of head-ref.
# Fails open: an unknown or missing base, or any lookup error, prints true.
set -euo pipefail

component="${1:?usage: changed-components.sh <component> <base-ref> [head-ref]}"
base="${2:-}"
head="${3:-HEAD}"

changed_true() {
	echo "changed-components: ${component}: $1" >&2
	echo true
	exit 0
}

[ -n "$base" ] || changed_true "no base ref, publishing"
git rev-parse -q --verify "${base}^{commit}" >/dev/null || changed_true "base ${base} not found, publishing"
git rev-parse -q --verify "${head}^{commit}" >/dev/null || changed_true "head ${head} not found, publishing"

# Go-built components: the exact source and embed files of the binary, every OS.
cmd=""
extra=()
case "$component" in
mcp)
	cmd=./cmd/levelrail-mcp
	extra=(scripts/build-mcp-npm-packages.sh scripts/render-mcp-server-json.mjs server.json deploy/packaging/npm/README-mcp.md)
	;;
cli)
	cmd=./cmd/levelrail-cli
	extra=(scripts/build-npm-packages.sh deploy/packaging/npm/ deploy/packaging/homebrew/ scripts/render-homebrew-formula.sh scripts/release/gen-windows-manifests.sh scripts/release/gen-aur-nix.sh scripts/release/build-cli-packages.sh)
	;;
agent)
	cmd=./cmd/levelrail-agent
	extra=(deploy/packaging/nfpm/ deploy/docker/Dockerfile.levelrail-agent)
	;;
terraform)
	extra=(integrations/terraform-provider/)
	;;
*)
	echo "changed-components: unknown component ${component}" >&2
	exit 2
	;;
esac

# A change to the detector itself republishes everything once.
extra+=(scripts/release/changed-components.sh)

diff_files="$(git diff --name-only "$base" "$head")" || changed_true "git diff failed, publishing"
[ -n "$diff_files" ] || {
	echo false
	exit 0
}

inputs="$(printf '%s\n' "${extra[@]}")"
if [ -n "$cmd" ]; then
	# go list reports directories under $PWD, so match the path git reports.
	root="$(git rev-parse --show-toplevel)"
	cd "$root"
	# shellcheck disable=SC2016 # a Go template, not a shell expansion
	tmpl='{{if not .Standard}}{{$d := .Dir}}{{range .GoFiles}}{{$d}}/{{.}}
{{end}}{{range .CgoFiles}}{{$d}}/{{.}}
{{end}}{{range .IgnoredGoFiles}}{{$d}}/{{.}}
{{end}}{{range .EmbedFiles}}{{$d}}/{{.}}
{{end}}{{end}}'
	go_files=""
	for goos in linux darwin windows; do
		out="$(GOOS="$goos" go list -deps -f "$tmpl" "$cmd")" || changed_true "go list failed for ${goos}, publishing"
		go_files+="${out}"$'\n'
	done
	# Module files outside this repo (dependencies) are covered by go.mod and go.sum.
	go_files="$(printf '%s\n' "$go_files" | sed -n "s#^${root}/##p" | sort -u)"
	[ -n "$go_files" ] || changed_true "no Go inputs resolved, publishing"
	inputs+=$'\n'"go.mod"$'\n'"go.sum"$'\n'"kit/go.mod"$'\n'"kit/go.sum"$'\n'"${go_files}"
fi

exact="$(printf '%s\n' "$inputs" | grep -v '/$' | grep -v '^$' || true)"
hit="$(printf '%s\n' "$diff_files" | grep -Fx -f <(printf '%s\n' "$exact") | head -1 || true)"
[ -z "$hit" ] || changed_true "$hit"
while IFS= read -r prefix; do
	[ -n "$prefix" ] || continue
	hit="$(printf '%s\n' "$diff_files" | grep -F -- "$prefix" | grep -m1 "^${prefix}" || true)"
	[ -z "$hit" ] || changed_true "$hit"
done < <(printf '%s\n' "$inputs" | grep '/$' || true)

echo "changed-components: ${component}: no input changed since ${base}" >&2
echo false
