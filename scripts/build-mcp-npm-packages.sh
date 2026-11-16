#!/usr/bin/env bash
# Assemble the levelrail-mcp npm packages: one binary package per platform plus
# the launcher that depends on them. Kept apart from build-npm-packages.sh so the
# CLI packages never change because of it.
# Usage: scripts/build-mcp-npm-packages.sh <tag> <dist-dir> <out-dir>
# <dist-dir> holds levelrail-mcp-<os>-<arch>[.exe] binaries.
set -euo pipefail

tag="${1:?usage: build-mcp-npm-packages.sh <tag> <dist-dir> <out-dir>}"
dist="${2:?usage: build-mcp-npm-packages.sh <tag> <dist-dir> <out-dir>}"
out="${3:?usage: build-mcp-npm-packages.sh <tag> <dist-dir> <out-dir>}"
root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
version="${tag#v}"
keywords='"keywords": ["levelrail", "mcp", "mcp-server", "model-context-protocol", "self-hosted", "deploy", "devops"],'

platforms="linux:amd64:linux:x64 linux:arm64:linux:arm64 darwin:amd64:darwin:x64 darwin:arm64:darwin:arm64 windows:amd64:win32:x64 windows:arm64:win32:arm64"

rm -rf "$out"
mkdir -p "$out"
deps=""

for p in $platforms; do
	IFS=: read -r goos goarch npm_os npm_cpu <<<"$p"
	name="levelrail-mcp-${goos}-${goarch}"
	ext=""
	[ "$goos" = "windows" ] && ext=".exe"
	src="$dist/${name}${ext}"
	[ -f "$src" ] || { echo "missing binary: $src" >&2; exit 1; }

	dir="$out/$name"
	mkdir -p "$dir/bin"
	cp "$src" "$dir/bin/levelrail-mcp${ext}"
	chmod 755 "$dir/bin/levelrail-mcp${ext}"
	cp "$root/LICENSE" "$dir/LICENSE"
	cat >"$dir/README.md" <<EOF3
# ${name}

Prebuilt Levelrail MCP server binary for ${goos}/${goarch}. Do not install this package directly: it is pulled in by [levelrail-mcp](https://www.npmjs.com/package/levelrail-mcp).

Apache 2.0.
EOF3
	cat >"$dir/package.json" <<EOF
{
  "name": "${name}",
  "version": "${version}",
  "description": "Prebuilt Levelrail MCP server binary for ${goos}/${goarch}. Installed automatically by levelrail-mcp.",
  "license": "Apache-2.0",
  "repository": { "type": "git", "url": "git+https://github.com/glincker/levelrail.git", "directory": "deploy/packaging/npm" },
  "homepage": "https://levelrail.com",
  "bugs": { "url": "https://github.com/glincker/levelrail/issues" },
  ${keywords}
  "publishConfig": { "access": "public", "provenance": true },
  "os": ["${npm_os}"],
  "cpu": ["${npm_cpu}"],
  "files": ["bin", "README.md", "LICENSE"]
}
EOF
	deps="${deps}    \"${name}\": \"${version}\",
"
done

main="$out/levelrail-mcp"
mkdir -p "$main/bin"
cp "$root/deploy/packaging/npm/levelrail-mcp.js" "$main/bin/levelrail-mcp.js"
chmod 755 "$main/bin/levelrail-mcp.js"
cp "$root/deploy/packaging/npm/README-mcp.md" "$main/README.md"
cp "$root/LICENSE" "$main/LICENSE"
deps="${deps%,
}"
# mcpName must equal the name in server.json: the MCP Registry checks it.
cat >"$main/package.json" <<EOF
{
  "name": "levelrail-mcp",
  "version": "${version}",
  "mcpName": "io.github.glincker/levelrail",
  "description": "MCP server for Levelrail, a self-hosted deployment platform",
  "license": "Apache-2.0",
  "repository": { "type": "git", "url": "git+https://github.com/glincker/levelrail.git", "directory": "deploy/packaging/npm" },
  "homepage": "https://levelrail.com",
  "author": { "name": "Glincker", "url": "https://levelrail.com" },
  "bugs": { "url": "https://github.com/glincker/levelrail/issues" },
  ${keywords}
  "publishConfig": { "access": "public", "provenance": true },
  "bin": { "levelrail-mcp": "bin/levelrail-mcp.js" },
  "files": ["bin", "README.md", "LICENSE"],
  "engines": { "node": ">=18" },
  "optionalDependencies": {
${deps}
  }
}
EOF

echo "built levelrail-mcp npm packages for ${version} in ${out}"
