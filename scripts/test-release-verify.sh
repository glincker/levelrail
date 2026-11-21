#!/usr/bin/env bash
# Checks verify-assets.sh against a fake gh on PATH: complete set passes, a missing sigstore bundle fails.
set -euo pipefail
root="$(cd "$(dirname "$0")/.." && pwd)"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT
mkdir "$tmp/bin"

cat >"$tmp/bin/gh" <<'FAKE'
#!/usr/bin/env bash
cat "$FAKE_ASSETS"
FAKE
chmod +x "$tmp/bin/gh"
export PATH="$tmp/bin:$PATH" FAKE_ASSETS="$tmp/assets"

{
	echo checksums.txt; echo checksums.txt.sigstore.json; echo release-manifest.json
	for a in amd64 arm64; do
		echo "levelrail-linux-$a"; echo "levelrail-agent-linux-$a"
		echo "levelrail-agent-linux-$a.deb"; echo "levelrail-agent-linux-$a.rpm"
		echo "levelrail-agent-darwin-$a"
		echo "levelrail-cli-linux-$a"; echo "levelrail-cli-darwin-$a"; echo "levelrail-cli-windows-$a.exe"
	done
} >"$tmp/assets"

"$root/scripts/release/verify-assets.sh" v9.9.9 >/dev/null
echo "ok: complete set passes"

grep -v sigstore "$tmp/assets" >"$tmp/short"
FAKE_ASSETS="$tmp/short"
export FAKE_ASSETS
if out="$("$root/scripts/release/verify-assets.sh" v9.9.9 2>&1)"; then
	echo "FAIL: incomplete set passed" >&2; exit 1
fi
case "$out" in
*checksums.txt.sigstore.json*) echo "ok: missing bundle reported" ;;
*) echo "FAIL: wrong report: $out" >&2; exit 1 ;;
esac

# failed-jobs.sh lists failed jobs with a link and the failing step, and skips the rest.
cat >"$tmp/bin/gh" <<'FAKE'
#!/usr/bin/env bash
cat "$FAKE_JOBS"
FAKE
cat >"$tmp/jobs.json" <<'JSON'
{"jobs":[
{"name":"Build","conclusion":"success","html_url":"https://x/1","steps":[]},
{"name":"Merge and tag image","conclusion":"failure","html_url":"https://x/2","steps":[{"name":"Set up job","conclusion":"success"},{"name":"Log in","conclusion":"failure"}]},
{"name":"Skipped","conclusion":"skipped","html_url":"https://x/3","steps":[]}
]}
JSON
out="$(FAKE_JOBS="$tmp/jobs.json" "$root/scripts/release/failed-jobs.sh" 1 --repo o/r)"
case "$out" in
'- [Merge and tag image](https://x/2): failed at step "Log in"') echo "ok: failed job listed with step" ;;
*) echo "FAIL: wrong failed-jobs output: $out" >&2; exit 1 ;;
esac
