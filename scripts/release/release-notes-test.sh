#!/usr/bin/env bash
# Offline check for polish-notes.sh using fixture data, no network.
# Usage: scripts/release/release-notes-test.sh
set -euo pipefail

here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT
fail() { echo "FAIL: $*" >&2; exit 1; }
t=$'\t'

# fixture <name> <prerelease> <prev>: prints the fixture dir.
fixture() {
	local d="$work/$1"
	mkdir -p "$d"
	printf '%s\n' "$2" >"$d/prerelease"
	[ -n "$3" ] && printf '%s\n' "$3" >"$d/prev_tag"
	echo "abc1234def" >"$d/sha"
	printf '<!-- release-notes:start -->\nhand text\n<!-- release-notes:end -->\n\nmanual footer\n' >"$d/body.md"
	: >"$d/commits.tsv"
	echo "$d"
}
run() { POLISH_FIXTURE_DIR="$1" GITHUB_REPOSITORY=glincker/levelrail "$here/polish-notes.sh" "$2" --dry-run 2>/dev/null; }

echo "== stable with contributors, first-timer and bots"
d="$(fixture stable false v1.9.0)"
{
	printf 'alice%sfeat(api): add rollout\n' "$t"
	printf 'alice%sfix: stop crash\n' "$t"
	printf 'bob%sfix(security): harden token check\n' "$t"
	printf 'dependabot[bot]%sfix(deps): bump x\n' "$t"
	printf '%schore: squash merge without login\n' "$t"
	printf 'carol%sfeat!: drop v1 route\n' "$t"
} >"$d/commits.tsv"
printf 'alice\nbob\n' >"$d/known_logins.txt"
out="$(run "$d" v2.0.0)"
grep -q 'raw.githubusercontent.com/glincker/levelrail/v2.0.0/' <<<"$out" || fail "logos pinned to tag"
grep -q '^- @carol (first contribution)' <<<"$out" || fail "first-timer mark"
grep -q '^- @alice$' <<<"$out" || fail "returning contributor"
grep -q 'dependabot' <<<"$out" && fail "bot listed"
grep -q 'Breaking changes' <<<"$out" || fail "breaking called out"
grep -q 'compare/v1.9.0...v2.0.0' <<<"$out" || fail "compare link"
grep -q 'manual footer' <<<"$out" || fail "hand text kept"
grep -q 'harden security' <<<"$out" || fail "security line"

echo "== idempotent re-run keeps one copy of each block"
printf '%s\n' "$out" >"$d/body.md"
out2="$(run "$d" v2.0.0)"
[ "$out" = "$out2" ] || fail "second render differs"
[ "$(grep -c 'polish-header:start' <<<"$out2")" = 1 ] || fail "duplicate header"

echo "== prerelease uses any previous tag and notes the channel"
d="$(fixture pre true v2.0.0)"
out="$(run "$d" v2.1.0-rc.1)"
grep -q 'This is a prerelease' <<<"$out" || fail "prerelease note"
grep -q 'compare/v2.0.0...v2.1.0-rc.1' <<<"$out" || fail "prerelease compare"

echo "== first release has no compare range and no empty sections"
d="$(fixture first false '')"
out="$(run "$d" v0.1.0)"
grep -q 'commits/v0.1.0' <<<"$out" || fail "first release changelog link"
grep -q '^## Contributors' <<<"$out" && fail "empty contributors heading"
grep -q '^## Highlights' <<<"$out" && fail "empty highlights heading"

echo "== sections already in the body are not repeated"
d="$(fixture dup false v2.1.0)"
printf '## Install\n\ntext\n\n## Contributors\n\nx\n\nFull changelog: y\n' >"$d/body.md"
printf 'alice%sfeat: a\n' "$t" >"$d/commits.tsv"
out="$(run "$d" v2.2.0)"
[ "$(grep -c '^## Install' <<<"$out")" = 1 ] || fail "install repeated"
[ "$(grep -c '^## Contributors' <<<"$out")" = 1 ] || fail "contributors repeated"
grep -q 'polish-changelog' <<<"$out" && fail "changelog repeated"

echo "== contributor cap and size trimming"
d="$(fixture big false v2.9.0)"
for i in $(seq 1 40); do printf 'user%s%sfeat: change %s\n' "$i" "$t" "$i"; done >"$d/commits.tsv"
out="$(run "$d" v3.0.0)"
grep -q '^- +10 more' <<<"$out" || fail "cap marker"
small="$(POLISH_MAX_BODY=5000 POLISH_FIXTURE_DIR="$d" "$here/polish-notes.sh" v3.0.0 --dry-run 2>/dev/null)"
[ "${#small}" -le 5000 ] || fail "over the size limit: ${#small}"
grep -q 'hand text' <<<"$small" || fail "hand text lost when trimming"

echo "ok"
