#!/usr/bin/env bash
# Checks the Homebrew formula and npm package generators against a real CLI build.
# Usage: scripts/test-packaging.sh
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT
tag="v9.9.9-test.1"

fail() { echo "FAIL: $*" >&2; exit 1; }

sha256() { if command -v sha256sum >/dev/null; then sha256sum "$@"; else shasum -a 256 "$@"; fi; }

mkdir -p "$work/dist"
(cd "$root" && CGO_ENABLED=0 go build -o "$work/host-cli" ./cmd/levelrail-cli)
for p in linux-amd64 linux-arm64 darwin-amd64 darwin-arm64 windows-amd64.exe windows-arm64.exe; do
	cp "$work/host-cli" "$work/dist/levelrail-cli-$p"
done
(cd "$work/dist" && sha256 -- * | sed 's/ \*/  /' >checksums.txt)

echo "== homebrew formula"
"$root/scripts/render-homebrew-formula.sh" "$tag" "$work/dist/checksums.txt" >"$work/levelrail-cli.rb"
grep -q '@' "$work/levelrail-cli.rb" && fail "unreplaced placeholder in formula"
grep -q 'version "9.9.9-test.1"' "$work/levelrail-cli.rb" || fail "formula version"
grep -q "download/$tag/levelrail-cli-darwin-arm64" "$work/levelrail-cli.rb" || fail "formula url"
want="$(awk '$2 == "levelrail-cli-linux-amd64" { print $1 }' "$work/dist/checksums.txt")"
grep -q "$want" "$work/levelrail-cli.rb" || fail "formula checksum"
if command -v ruby >/dev/null; then ruby -c "$work/levelrail-cli.rb" >/dev/null || fail "formula is not valid ruby"; fi
if "$root/scripts/render-homebrew-formula.sh" "$tag" /dev/null >/dev/null 2>&1; then fail "missing checksum should fail"; fi

echo "== npm packages"
"$root/scripts/build-npm-packages.sh" "$tag" "$work/dist" "$work/npm" >/dev/null
for d in "$work"/npm/*/; do node -e 'JSON.parse(require("fs").readFileSync(process.argv[1]+"package.json"))' "$d" || fail "bad package.json in $d"; done
node -e '
const p = require(process.argv[1]);
const deps = Object.keys(p.optionalDependencies);
if (deps.length !== 6) { console.error("want 6 platform deps, got", deps.length); process.exit(1); }
if (p.version !== "9.9.9-test.1") { console.error("version", p.version); process.exit(1); }
if (!deps.includes("levelrail-cli-win32-x64") && !deps.includes("levelrail-cli-windows-amd64")) process.exit(1);
' "$work/npm/levelrail-cli/package.json" || fail "main package.json"
for d in "$work"/npm/*/; do (cd "$d" && npm pack --dry-run --silent >/dev/null 2>&1) || fail "npm pack failed for $d"; done

echo "== launcher resolves the platform package"
case "$(uname -s)" in Linux) goos=linux ;; Darwin) goos=darwin ;; *) echo "skip launcher run on $(uname -s)"; echo "OK"; exit 0 ;; esac
case "$(uname -m)" in x86_64 | amd64) goarch=amd64 ;; *) goarch=arm64 ;; esac
mkdir -p "$work/proj/node_modules"
cp -R "$work/npm/levelrail-cli" "$work/proj/node_modules/levelrail-cli"
cp -R "$work/npm/levelrail-cli-$goos-$goarch" "$work/proj/node_modules/levelrail-cli-$goos-$goarch"
out="$(node "$work/proj/node_modules/levelrail-cli/bin/levelrail-cli.js" help)" || fail "launcher exited non-zero"
echo "$out" | grep -q "scriptable client" || fail "launcher output: $out"
node "$work/proj/node_modules/levelrail-cli/bin/levelrail-cli.js" no-such-command >/dev/null 2>&1 && fail "exit code not propagated"
rm -rf "$work/proj/node_modules/levelrail-cli-$goos-$goarch"
missing="$(node "$work/proj/node_modules/levelrail-cli/bin/levelrail-cli.js" help 2>&1 || true)"
echo "$missing" | grep -q "package is missing" || fail "missing-platform message: $missing"

echo "OK"
