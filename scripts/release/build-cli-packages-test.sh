#!/usr/bin/env bash
# Runs build-cli-packages.sh with a stub nfpm and checks names, env and checksums.
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT
mkdir -p "$tmp/bin"

cat > "$tmp/nfpm" <<'STUB'
#!/usr/bin/env bash
target=""
while [ $# -gt 0 ]; do
	[ "$1" = "--target" ] && target="$2"
	shift
done
[ -f pkg/levelrail-cli ] || exit 3
printf '%s %s' "$VERSION" "$ARCH" > "$target"
STUB
chmod +x "$tmp/nfpm"
printf 'a' > "$tmp/bin/levelrail-cli-linux-amd64"
printf 'b' > "$tmp/bin/levelrail-cli-linux-arm64"

NFPM="$tmp/nfpm" "$root/scripts/release/build-cli-packages.sh" v1.4.0-beta.1 "$tmp/bin" "$tmp/out"

for f in levelrail-cli-linux-amd64.deb levelrail-cli-linux-amd64.rpm levelrail-cli-linux-arm64.deb levelrail-cli-linux-arm64.rpm; do
	[ -f "$tmp/out/$f" ] || { echo "missing $f" >&2; exit 1; }
	grep -q "^[0-9a-f]\{64\}  $f$" "$tmp/out/package-checksums.txt" || { echo "no checksum for $f" >&2; exit 1; }
done
[ "$(cat "$tmp/out/levelrail-cli-linux-arm64.rpm")" = "1.4.0-beta.1 arm64" ] || { echo "bad version or arch" >&2; exit 1; }

rm "$tmp/bin/levelrail-cli-linux-arm64"
if NFPM="$tmp/nfpm" "$root/scripts/release/build-cli-packages.sh" v1.4.0 "$tmp/bin" "$tmp/out2" 2>/dev/null; then
	echo "expected failure on a missing binary" >&2
	exit 1
fi
echo "ok"
