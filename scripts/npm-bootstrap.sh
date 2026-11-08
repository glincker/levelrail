#!/usr/bin/env bash
# One-time npm setup for the CLI packages: publish them once so they exist,
# register this repo's release workflow as each package's trusted publisher,
# then switch on the release job. Run it yourself, logged in to npm.
# Usage: scripts/npm-bootstrap.sh [--dry-run] [--yes] [tag]
set -euo pipefail

repo="glincker/levelrail"
workflow="release.yml"
environment="npm-publish"
npm_version="11.18.0"

dry=0
yes=0
tag=""
for arg in "$@"; do
	case "$arg" in
	--dry-run) dry=1 ;;
	--yes) yes=1 ;;
	-*) echo "unknown flag: $arg" >&2; exit 2 ;;
	*) tag="$arg" ;;
	esac
done

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
npm() { command npx -y "npm@${npm_version}" "$@"; }

[ -n "$tag" ] || tag="$(gh release list --repo "$repo" --limit 1 --json tagName --jq '.[0].tagName')"
version="${tag#v}"
dist_tag=latest
case "$tag" in *-*) dist_tag=beta ;; esac

work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT

echo "Release:  $tag (npm version $version, dist-tag $dist_tag)"
gh release download "$tag" --repo "$repo" --pattern 'levelrail-cli-*' --dir "$work/dist" >/dev/null
"$root/scripts/build-npm-packages.sh" "$tag" "$work/dist" "$work/out" >/dev/null

# Platform packages first, then the launcher, then the forwarders that
# depend on the launcher. Entries are output directories.
packages=()
for d in "$work"/out/levelrail-cli-*/ "$work"/out/levelrail-cli/ "$work"/out/levelrail/ "$work"/out/levelrail-scoped/; do
	packages+=("$(basename "$d")")
done
pkg_name() { node -p "require('$work/out/$1/package.json').name"; }

echo "Packages: ${packages[*]} (levelrail-scoped publishes as @levelrail/cli)"
echo "Trusted publisher: github.com/$repo, workflow $workflow, environment $environment"
if [ "$dry" -eq 0 ] && [ "$yes" -eq 0 ]; then
	read -r -p "Publish these and register the trusted publisher? [y/N] " answer
	[ "$answer" = "y" ] || { echo "aborted"; exit 1; }
fi

if [ "$dry" -eq 0 ]; then
	npm whoami >/dev/null 2>&1 || npm login
	echo "npm user: $(npm whoami)"
fi

for dir in "${packages[@]}"; do
	name="$(pkg_name "$dir")"
	if npm view "${name}@${version}" version >/dev/null 2>&1; then
		echo "skip     ${name}@${version} (already on npm)"
		continue
	fi
	echo "publish  ${name}@${version}"
	if [ "$dry" -eq 1 ]; then
		(cd "$work/out/$dir" && npm publish --dry-run --access public --tag "$dist_tag" >/dev/null)
	else
		(cd "$work/out/$dir" && npm publish --access public --tag "$dist_tag")
	fi
done

failed=0
for dir in "${packages[@]}"; do
	name="$(pkg_name "$dir")"
	if npm trust list "$name" 2>/dev/null | grep -q "$repo"; then
		echo "skip     trust for $name (already registered)"
		continue
	fi
	echo "trust    $name"
	args=(trust github "$name" --file "$workflow" --repo "$repo" --env "$environment" --allow-publish -y)
	[ "$dry" -eq 1 ] && args+=(--dry-run)
	if ! npm "${args[@]}"; then
		echo "WARNING: could not register $name (already registered, or the package is missing)" >&2
		failed=$((failed + 1))
	fi
done

if [ "$failed" -ne 0 ]; then
	echo "$failed package(s) not registered: not enabling the release job. Fix and re-run; finished steps are skipped." >&2
	exit 1
fi

if [ "$dry" -eq 1 ]; then
	echo "dry run complete: would run: gh variable set NPM_PUBLISH_ENABLED --body true --repo $repo"
	exit 0
fi
gh variable set NPM_PUBLISH_ENABLED --body true --repo "$repo"
echo "done: the next release publishes to npm with no token."
