#!/usr/bin/env bash
# Builds two real control plane binaries and asserts release-gate.sh passes the good path and refuses the bad ones.
set -euo pipefail
root="$(cd "$(dirname "$0")/.." && pwd)"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT
cd "$root"

pkg=github.com/GLINCKER/levelrail/internal/version.Version
CGO_ENABLED=0 go build -ldflags "-X $pkg=v9.9.8" -o "$tmp/prev" ./cmd/levelrail
CGO_ENABLED=0 go build -ldflags "-X $pkg=v9.9.9" -o "$tmp/cand" ./cmd/levelrail
mkdir "$tmp/no-fixtures"
schema="$("$tmp/cand" version --json | jq -r .schema_version)"
"$tmp/cand" version --json >"$tmp/manifest.json"
gate="$root/scripts/release-gate.sh"

expect_fail() { # name wanted-text args...
	local name="$1" want="$2"
	shift 2
	if out="$("$gate" "$@" 2>&1)"; then
		echo "FAIL: $name passed but must be refused" >&2
		exit 1
	fi
	case "$out" in
	*"$want"*) echo "ok: $name refused" ;;
	*) echo "FAIL: $name refused for the wrong reason (wanted '$want'):" >&2; echo "$out" >&2; exit 1 ;;
	esac
}

"$gate" --candidate "$tmp/cand" --tag v9.9.9 --manifest "$tmp/manifest.json" --prev-binary "$tmp/prev" --prev-tag v9.9.8 --fixtures "$tmp/no-fixtures"
echo "ok: good path passed"

"$gate" --candidate "$tmp/cand" --tag v9.9.9 >/dev/null 2>&1
echo "ok: first release (no previous) passes with a warning"

expect_fail "lower schema than the previous release" "not monotonic" \
	--candidate "$tmp/cand" --tag v9.9.9 --prev-binary "$tmp/prev" --prev-tag v9.9.8 --prev-schema $((schema + 5)) --fixtures "$tmp/no-fixtures"

printf '{"version":"v9.9.9","schema_version":%d}\n' $((schema + 1)) >"$tmp/bad-manifest.json"
expect_fail "manifest that disagrees with the binary" "release manifest does not describe" \
	--candidate "$tmp/cand" --tag v9.9.9 --manifest "$tmp/bad-manifest.json"

printf '{"version":"v1.0.0","schema_version":%d}\n' "$schema" >"$tmp/wrong-tag.json"
expect_fail "manifest for another tag" "release manifest does not describe" \
	--candidate "$tmp/cand" --tag v9.9.9 --manifest "$tmp/wrong-tag.json"

printf '#!/bin/sh\necho "not a release" >&2\nexit 1\n' >"$tmp/broken"
chmod +x "$tmp/broken"
expect_fail "candidate that does not report a schema" "does not report its schema" --candidate "$tmp/broken" --tag v9.9.9
echo "release gate script tests passed"
