#!/usr/bin/env bash
# Self test for scripts/dev-bootstrap.sh with a stub npm and a fake lockfile.
# Touches only a temp dir; no real npm runs.
set -uo pipefail
src="$(cd "$(dirname "$0")/.." && pwd)"

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT
repo="$tmp/repo"
mkdir -p "$repo/web" "$repo/scripts" "$tmp/bin"
cp "$src/scripts/dev-bootstrap.sh" "$repo/scripts/"
git -C "$repo" init -q
echo '{"name":"fake"}' >"$repo/web/package.json"
echo '{"lockfileVersion":3}' >"$repo/web/package-lock.json"
cat >"$tmp/bin/npm" <<'STUB'
#!/bin/sh
echo run >>"$NPM_COUNT_FILE"
sleep 1
mkdir -p node_modules/.bin
echo marker >node_modules/marker
STUB
chmod +x "$tmp/bin/npm"
export PATH="$tmp/bin:$PATH" NPM_COUNT_FILE="$tmp/npm.count"
export LEVELRAIL_GOLDEN_DIR="$tmp/golden" LEVELRAIL_GOLDEN_LOCK_WAIT=30
boot="$repo/scripts/dev-bootstrap.sh"
fail=0
check() { if ! "$@"; then echo "FAIL: $*"; fail=1; fi; }

# Two concurrent runs build the golden copy once.
"$boot" >"$tmp/o1" 2>&1 &
p1=$!
"$boot" >"$tmp/o2" 2>&1 &
p2=$!
wait "$p1"; check test $? -eq 0
wait "$p2"; check test $? -eq 0
check test "$(wc -l <"$tmp/npm.count" | tr -d ' ')" = 1
check test -f "$repo/web/node_modules/marker"
check test ! -L "$repo/web/node_modules"
cat "$tmp/o1" "$tmp/o2"

# A second run is a no-op.
out="$("$boot" 2>&1)"
check test "$(echo "$out" | wc -l | tr -d ' ')" = 1
check grep -q "already current" <<<"$out"
check test "$(wc -l <"$tmp/npm.count" | tr -d ' ')" = 1

# A changed lockfile builds a second golden copy.
echo '{"lockfileVersion":3,"x":1}' >"$repo/web/package-lock.json"
"$boot" >/dev/null 2>&1; check test $? -eq 0
check test "$(wc -l <"$tmp/npm.count" | tr -d ' ')" = 2

# Prune keeps the current hash and removes only old hex-named copies.
old="$tmp/golden/0123456789abcdef"
mkdir -p "$old" "$tmp/golden/keepme"
touch -t 202001010000 "$old" "$tmp/golden/keepme"
out="$("$boot" --prune 2>&1)"
check test ! -d "$old"
check test -d "$tmp/golden/keepme"
check test "$(find "$tmp/golden" -mindepth 1 -maxdepth 1 -type d -name '[0-9a-f]*' | wc -l | tr -d ' ')" = 2
echo "$out"

# No lockfile exits 0.
rm "$repo/web/package-lock.json"
"$boot" >/dev/null 2>&1; check test $? -eq 0

[ "$fail" -eq 0 ] && echo "dev-bootstrap self test: ok"
exit "$fail"
