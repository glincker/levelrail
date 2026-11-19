#!/usr/bin/env bash
# Checks scripts/release/changed-components.sh against a throwaway repo laid out like this one.
set -euo pipefail
root="$(cd "$(dirname "$0")/.." && pwd)"
script="$root/scripts/release/changed-components.sh"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT
cd "$tmp"
export GOFLAGS=-mod=mod GOTOOLCHAIN=local

git init -q
git config user.email test@example.com
git config user.name test
mkdir -p cmd/levelrail-mcp internal/shared internal/cponly integrations/terraform-provider scripts/release
printf 'module example.com/lr\n\ngo 1.21\n' >go.mod
printf 'package shared\n\nconst V = 1\n' >internal/shared/shared.go
printf 'package shared\n\nfunc init() {}\n' >internal/shared/register.go
mkdir -p internal/shared/assets
echo hello >internal/shared/assets/hello.txt
echo extra >internal/shared/assets/extra.txt
echo keep >internal/shared/assets/keep.txt
mkdir -p internal/shared/runbooks deploy/packaging/npm
echo '# restart' >internal/shared/runbooks/restart.md
echo '# drain' >internal/shared/runbooks/drain.md
printf 'package shared\n\nimport "embed"\n\n//go:embed runbooks/*.md\nvar Runbooks embed.FS\n' >internal/shared/runbooks.go
echo 'launcher v1' >deploy/packaging/npm/levelrail-mcp.js
echo 'license v1' >LICENSE
printf 'package shared\n\nimport "embed"\n\n//go:embed assets\nvar Assets embed.FS\n' >internal/shared/assets.go
printf 'package cponly\n\nconst V = 1\n' >internal/cponly/cponly.go
printf 'package main\n\nimport _ "example.com/lr/internal/shared"\n\nfunc main() {}\n' >cmd/levelrail-mcp/main.go
echo v1 >integrations/terraform-provider/VERSION
cp "$script" scripts/release/changed-components.sh
git add -A
git commit -qm base
git tag v1.0.0

# expect_hit checks the detector named this file, so an early fail-open cannot pass the case.
expect_hit() {
	local file="$1"
	shift
	local got why
	got="$("$script" "$@" 2>"$tmp/why")"
	why="$(cat "$tmp/why")"
	if [ "$got" != "true" ] || [[ "$why" != *"$file"* ]]; then
		echo "FAIL: changed-components.sh $* printed '$got' ($why), want true for $file" >&2
		exit 1
	fi
}

go_lists() {
	go list ./cmd/levelrail-mcp >/dev/null || {
		echo "FAIL: go list broke in the fixture, a later case would pass by failing open" >&2
		exit 1
	}
}

expect() {
	local want="$1"
	shift
	local got
	got="$("$script" "$@" 2>/dev/null)"
	if [ "$got" != "$want" ]; then
		echo "FAIL: changed-components.sh $* printed '$got', want '$want'" >&2
		exit 1
	fi
}

expect false mcp v1.0.0
expect false terraform v1.0.0
expect true mcp ""
expect true mcp v0.0.1
echo "ok: no change, missing base"

echo 'const W = 2' >>internal/cponly/cponly.go
git commit -qam "control plane only"
expect false mcp v1.0.0
expect false terraform v1.0.0
echo "ok: a package the binary does not import is ignored"

echo 'const W = 2' >>internal/shared/shared.go
git commit -qam "shared"
expect true mcp v1.0.0
expect false terraform v1.0.0
echo "ok: an imported package marks the binary changed"

git tag v1.0.1
echo v2 >integrations/terraform-provider/VERSION
git commit -qam "provider"
expect false mcp v1.0.1
expect true terraform v1.0.1
echo "ok: directory inputs"

git tag v1.0.2
git rm -q internal/shared/register.go
git commit -qm "drop an init file"
go_lists
expect_hit internal/shared/register.go mcp v1.0.2
echo "ok: a deleted init file marks the binary changed"

git tag v1.0.3
git rm -q internal/shared/assets/extra.txt
git commit -qm "drop an embedded file"
go_lists
expect_hit internal/shared/assets/extra.txt mcp v1.0.3
echo "ok: a deleted embedded file marks the binary changed"

git tag v1.0.5
mkdir -p internal/cponly/old
echo x >internal/cponly/old/gone.txt
git add -A
git commit -qm "control plane file"
git tag v1.0.6
git rm -q internal/cponly/old/gone.txt
git commit -qm "drop a control plane file"
expect false mcp v1.0.6
echo "ok: a deletion outside the imported packages is ignored"

git tag v1.0.7
git mv integrations/terraform-provider/VERSION moved-version.txt
git commit -qm "move a provider file out"
expect_hit integrations/terraform-provider/VERSION terraform v1.0.7
echo "ok: a file renamed out of an input marks the component changed"

git tag v1.0.8
git mv internal/shared/assets/hello.txt internal/hello.txt
git commit -qm "move an embedded file out"
go_lists
expect_hit internal/shared/assets/hello.txt mcp v1.0.8
echo "ok: a file renamed out of an imported package marks the binary changed"

git tag v1.0.9
accented=integrations/terraform-provider/$'caf\303\251'
mkdir -p "$accented"
echo x >"$accented/notes.txt"
git add -A
git commit -qm "non-ASCII path"
expect_hit "$accented/notes.txt" terraform v1.0.9
echo "ok: a non-ASCII path matches"

git tag v1.0.10
git rm -q internal/shared/runbooks/drain.md
git commit -qm "drop a runbook"
go_lists
expect_hit internal/shared/runbooks/drain.md mcp v1.0.10
echo "ok: a deleted markdown file matched by a go:embed pattern marks the binary changed"

git tag v1.0.11
echo 'launcher v2' >deploy/packaging/npm/levelrail-mcp.js
git commit -qam "launcher fix"
expect_hit deploy/packaging/npm/levelrail-mcp.js mcp v1.0.11
expect false terraform v1.0.11
echo "ok: a launcher-only change marks the MCP package changed"

git tag v1.0.12
echo 'license v2' >LICENSE
git commit -qam "license"
expect_hit LICENSE mcp v1.0.12
echo "ok: a LICENSE change marks the MCP package changed"

if "$script" nope v1.0.1 >/dev/null 2>&1; then
	echo "FAIL: unknown component accepted" >&2
	exit 1
fi
echo "ok: unknown component refused"
