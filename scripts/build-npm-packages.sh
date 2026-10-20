#!/usr/bin/env bash
# Assemble the npm packages for a release: one binary package per platform plus
# the levelrail-cli launcher that depends on them.
# Usage: scripts/build-npm-packages.sh <tag> <dist-dir> <out-dir>
# <dist-dir> holds the release's levelrail-cli-<os>-<arch>[.exe] binaries.
set -euo pipefail

tag="${1:?usage: build-npm-packages.sh <tag> <dist-dir> <out-dir>}"
dist="${2:?usage: build-npm-packages.sh <tag> <dist-dir> <out-dir>}"
out="${3:?usage: build-npm-packages.sh <tag> <dist-dir> <out-dir>}"
root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
version="${tag#v}"

platforms="linux:amd64:linux:x64 linux:arm64:linux:arm64 darwin:amd64:darwin:x64 darwin:arm64:darwin:arm64 windows:amd64:win32:x64 windows:arm64:win32:arm64"

rm -rf "$out"
mkdir -p "$out"
deps=""

for p in $platforms; do
	IFS=: read -r goos goarch npm_os npm_cpu <<<"$p"
	name="levelrail-cli-${goos}-${goarch}"
	ext=""
	[ "$goos" = "windows" ] && ext=".exe"
	src="$dist/${name}${ext}"
	[ -f "$src" ] || { echo "missing binary: $src" >&2; exit 1; }

	dir="$out/$name"
	mkdir -p "$dir/bin"
	cp "$src" "$dir/bin/levelrail-cli${ext}"
	chmod 755 "$dir/bin/levelrail-cli${ext}"
	cat >"$dir/package.json" <<EOF
{
  "name": "${name}",
  "version": "${version}",
  "description": "Prebuilt levelrail-cli binary for ${goos}/${goarch}",
  "license": "Apache-2.0",
  "repository": { "type": "git", "url": "git+https://github.com/glincker/levelrail.git" },
  "homepage": "https://levelrail.com",
  "os": ["${npm_os}"],
  "cpu": ["${npm_cpu}"],
  "files": ["bin"]
}
EOF
	deps="${deps}    \"${name}\": \"${version}\",
"
done

main="$out/levelrail-cli"
mkdir -p "$main/bin"
cp "$root/packaging/npm/levelrail-cli.js" "$main/bin/levelrail-cli.js"
chmod 755 "$main/bin/levelrail-cli.js"
cp "$root/packaging/npm/README.md" "$main/README.md"
cp "$root/LICENSE" "$main/LICENSE"
deps="${deps%,
}"
cat >"$main/package.json" <<EOF
{
  "name": "levelrail-cli",
  "version": "${version}",
  "description": "Command line client for Levelrail, a self-hosted deployment platform",
  "license": "Apache-2.0",
  "repository": { "type": "git", "url": "git+https://github.com/glincker/levelrail.git" },
  "homepage": "https://levelrail.com",
  "bugs": "https://github.com/glincker/levelrail/issues",
  "keywords": ["levelrail", "deploy", "paas", "self-hosted", "cli", "docker"],
  "bin": { "levelrail-cli": "bin/levelrail-cli.js" },
  "files": ["bin", "README.md", "LICENSE"],
  "optionalDependencies": {
${deps}
  }
}
EOF

echo "built npm packages for ${version} in ${out}"
