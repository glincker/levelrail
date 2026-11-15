#!/usr/bin/env bash
# Adds the header, highlights, install, verify, contributors and changelog
# blocks to a GitHub release body. Each block sits between marker comments, so
# a re-run replaces only our blocks and never touches text written by the
# release generator, release-please or a human.
#
# Usage: scripts/release/polish-notes.sh <tag> [--dry-run]
# Env: GH_TOKEN, GITHUB_REPOSITORY (default glincker/levelrail),
#      POLISH_FIXTURE_DIR (offline test input, see release-notes-test.sh),
#      POLISH_MAX_BODY (default 120000), POLISH_MAX_CONTRIBUTORS (default 30),
#      POLISH_MAX_COMMITS (default 3000), POLISH_SLEEP (default 0.2),
#      POLISH_BOT_REGEX, POLISH_TAGLINE, POLISH_DOCS_URL, POLISH_NPM_PACKAGE,
#      POLISH_DOCKERHUB_NS, POLISH_LOGO_* (repo paths of the brand files).
# Never fails the release: any gh error exits 0 with a warning.
set -uo pipefail

tag="${1:?usage: polish-notes.sh <tag> [--dry-run]}"
dry_run="${2:-}"
repo="${GITHUB_REPOSITORY:-glincker/levelrail}"
fx="${POLISH_FIXTURE_DIR:-}"
max_body="${POLISH_MAX_BODY:-120000}"
max_contrib="${POLISH_MAX_CONTRIBUTORS:-30}"
max_commits="${POLISH_MAX_COMMITS:-3000}"
nap="${POLISH_SLEEP:-0.2}"
bot_re="${POLISH_BOT_REGEX:-\[bot\]$|^(dependabot|github-actions|glinr-bot|renovate|web-flow|copilot)}"
tagline="${POLISH_TAGLINE:-Your own Vercel, on your own servers.}"
docs_url="${POLISH_DOCS_URL:-https://docs.levelrail.com}"
npm_pkg="${POLISH_NPM_PACKAGE:-levelrail-cli}"
hub_ns="${POLISH_DOCKERHUB_NS:-levelrail}"
# Brand files that exist in the repo today. Swap these paths when a new GLINR
# logo lands in docs/assets/brand.
logo_lr_light="${POLISH_LOGO_LEVELRAIL_LIGHT:-docs/assets/brand/wordmark-light.svg}"
logo_lr_dark="${POLISH_LOGO_LEVELRAIL_DARK:-docs/assets/brand/wordmark-dark.svg}"
logo_gl_light="${POLISH_LOGO_GLINR_LIGHT:-docs/assets/brand/glincker-dark.svg}"
logo_gl_dark="${POLISH_LOGO_GLINR_DARK:-docs/assets/brand/glincker-light.svg}"
image_ns="$(printf %s "${repo%%/*}" | tr '[:upper:]' '[:lower:]')"

warn() { echo "::warning::polish-notes: $*"; }
bail() {
	warn "$*, skipping"
	exit 0
}
work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT

# gh GET wrapper: fixtures never reach the network.
api() { gh api "$@" 2>/dev/null; }

# ---------------------------------------------------------------- inputs
read_body() {
	if [ -n "$fx" ]; then
		cat "$fx/body.md" 2>/dev/null || true
		return 0
	fi
	gh release view "$tag" --repo "$repo" --json body --jq .body
}

is_prerelease() {
	if [ -n "$fx" ]; then
		[ "$(cat "$fx/prerelease" 2>/dev/null || echo false)" = "true" ]
		return
	fi
	[ "$(gh release view "$tag" --repo "$repo" --json isPrerelease --jq .isPrerelease 2>/dev/null)" = "true" ]
}

# Previous tag: stable releases look back to the previous stable, prereleases
# to the previous release of any kind. Empty means first release.
previous_tag() {
	local list
	if [ -n "$fx" ]; then
		cat "$fx/prev_tag" 2>/dev/null || true
		return 0
	fi
	list="$(api "repos/$repo/releases?per_page=100" --paginate \
		--jq '.[] | select(.draft | not) | [.tag_name, .prerelease] | @tsv')" || return 0
	printf '%s\n' "$list" | awk -F'\t' -v t="$tag" -v pre="$1" '
		$1 == t { seen = 1; next }
		seen && (pre == "true" || $2 == "false") { print $1; exit }
		END { }
	'
}

# Tab separated: login, subject. One line per commit in the range.
read_commits() {
	if [ -n "$fx" ]; then
		cat "$fx/commits.tsv" 2>/dev/null || true
		return 0
	fi
	[ -n "$1" ] || return 0
	api "repos/$repo/compare/$1...$tag?per_page=100" --paginate \
		--jq '.commits[] | [(.author.login // ""), (.commit.message | split("\n")[0])] | @tsv' |
		head -n "$max_commits"
}

has_earlier_commit() {
	local login="$1" prev="$2"
	if [ -n "$fx" ]; then
		grep -qx "$login" "$fx/known_logins.txt" 2>/dev/null
		return
	fi
	sleep "$nap"
	[ -n "$(api "repos/$repo/commits?sha=$prev&author=$login&per_page=1" --jq '.[0].sha // empty')" ]
}

# Pin brand images to the tag when the file exists there, else to main.
asset_ref() {
	if [ -n "$fx" ]; then
		[ "$(cat "$fx/asset_at_tag" 2>/dev/null || echo true)" = "true" ] && echo "$tag" || echo main
		return 0
	fi
	if api "repos/$repo/contents/$logo_lr_light?ref=$tag" --silent >/dev/null; then
		echo "$tag"
	else
		echo main
	fi
}

# ---------------------------------------------------------------- blocks
# Each render_* writes the block body (markers added by wrap).
wrap() { printf '<!-- %s:start -->\n' "$1"; cat; printf '<!-- %s:end -->\n' "$1"; }

render_header() {
	local ref="$1" sha="$2" raw="https://raw.githubusercontent.com/$repo/$1"
	local v="${tag#v}" b="https://img.shields.io" q="?style=flat"
	local release_url="https://github.com/$repo/releases/tag/$tag"
	local pre_q=""
	is_prerelease && pre_q="&include_prereleases"
	{
		echo '<div align="center">'
		echo '<picture>'
		echo "  <source media=\"(prefers-color-scheme: dark)\" srcset=\"$raw/$logo_lr_dark\">"
		echo "  <img src=\"$raw/$logo_lr_light\" alt=\"Levelrail\" height=\"56\">"
		echo '</picture>'
		echo '<p><strong>'"$tagline"'</strong></p>'
		echo '<p>'
		echo "<a href=\"$release_url\"><img src=\"$b/badge/release-${v//-/--}-107292$q\" alt=\"Release $tag\"></a>"
		echo "<a href=\"$release_url\"><img src=\"$b/github/downloads/$repo/$tag/total$q\" alt=\"Downloads of this release\"></a>"
		echo "<a href=\"https://github.com/$repo/blob/$tag/LICENSE\"><img src=\"$b/github/license/$repo$q\" alt=\"License\"></a>"
		[ -n "$sha" ] && echo "<a href=\"https://github.com/$repo/commit/$sha/checks\"><img src=\"$b/github/check-runs/$repo/$sha$q&label=CI\" alt=\"CI status of the release commit\"></a>"
		echo "<a href=\"https://hub.docker.com/r/$hub_ns/levelrail\"><img src=\"$b/docker/pulls/$hub_ns/levelrail$q&logo=docker\" alt=\"Docker pulls\"></a>"
		echo "<a href=\"https://www.npmjs.com/package/$npm_pkg\"><img src=\"$b/npm/v/$npm_pkg$q&logo=npm&label=npm$pre_q\" alt=\"npm version\"></a>"
		if [ "${POLISH_SCORECARD:-$(scorecard_published)}" = "true" ]; then
			echo "<a href=\"https://scorecard.dev/viewer/?uri=github.com/$repo\"><img src=\"$b/ossf-scorecard/github.com/$repo$q&label=OpenSSF%20Scorecard\" alt=\"OpenSSF Scorecard\"></a>"
		fi
		echo "<a href=\"https://github.com/$repo/blob/$tag/docs/verify-a-release.md\"><img src=\"$b/badge/cosign-signed-107292$q\" alt=\"Signed with cosign\"></a>"
		echo "<a href=\"https://github.com/$repo/attestations\"><img src=\"$b/badge/SLSA-provenance-107292$q\" alt=\"SLSA build provenance attestation\"></a>"
		echo '</p>'
		echo '<p>'
		echo "<a href=\"${docs_url}\">Docs</a> &middot; <a href=\"https://github.com/$repo/discussions\">Discussions</a> &middot; <a href=\"https://github.com/$repo/issues\">Issues</a>"
		echo '</p>'
		echo '<a href="https://glinr.com">'
		echo '<picture>'
		echo "  <source media=\"(prefers-color-scheme: dark)\" srcset=\"$raw/$logo_gl_dark\">"
		echo "  <img src=\"$raw/$logo_gl_light\" alt=\"GLINR STUDIOS\" height=\"28\">"
		echo '</picture>'
		echo '</a>'
		echo '</div>'
	} | wrap polish-header
}

# True only when the repo already runs the OpenSSF Scorecard action.
scorecard_published() {
	if [ -n "$fx" ]; then echo false; return 0; fi
	grep -rqs 'ossf/scorecard-action' .github/workflows 2>/dev/null && echo true || echo false
}

plural() { if [ "$1" = 1 ]; then echo "$1 $2"; else echo "$1 $3"; fi; }

render_highlights() {
	local commits="$1" detail="$2" body="$3"
	local feat fix brk sec
	if [ -s "$commits" ]; then
		feat="$(grep -c -E $'\t(feat)(\\([^)]*\\))?!?:' "$commits")"
		fix="$(grep -c -E $'\t(fix)(\\([^)]*\\))?!?:' "$commits")"
		brk="$(grep -E $'\t[a-z]+(\\([^)]*\\))?!:' "$commits" | cut -f2 | head -5)"
		sec="$(cut -f2 "$commits" | grep -c -i -E 'security|cve-|vulnerab|harden')"
	else
		feat="$(printf '%s\n' "$body" | awk '/^### Features/{f=1;next} /^#/{f=0} f&&/^- /{n++} END{print n+0}')"
		fix="$(printf '%s\n' "$body" | awk '/^### Bug [Ff]ixes/{f=1;next} /^#/{f=0} f&&/^- /{n++} END{print n+0}')"
		brk="$(printf '%s\n' "$body" | grep -i -m5 -E 'breaking' || true)"
		sec=0
	fi
	# A single change is already the whole story, so no summary then.
	if [ $((feat + fix)) -lt 2 ] && [ -z "$brk" ] && [ "$sec" = 0 ]; then
		return 0
	fi
	{
		echo "## Highlights"
		echo
		if [ -n "$brk" ]; then
			echo "**Breaking changes** (read before upgrading):"
			echo
			printf '%s\n' "$brk" | sed 's/^/- /'
			echo
		fi
		echo "- $(plural "$feat" "new feature" "new features") and $(plural "$fix" "bug fix" "bug fixes") in this release."
		[ "$sec" -gt 0 ] && echo "- $sec changes that harden security. See the notes below for details."
		if [ "$detail" = "full" ] && [ -s "$commits" ]; then
			grep -E $'\tfeat(\\([^)]*\\))?!?:' "$commits" | cut -f2 | head -5 |
				sed -E 's/^feat(\([^)]*\))?!?: ?//; s/^/- New: /'
		fi
	} | wrap polish-highlights
}

render_install() {
	local pre="$1" up_cmd
	up_cmd="levelrail self-upgrade"
	{
		echo "## Install and upgrade"
		echo
		echo '```bash'
		echo "# Install this exact version"
		echo "curl -fsSL https://levelrail.com/install.sh | sudo env LEVELRAIL_VERSION=$tag sh"
		echo
		echo "# Upgrade an existing install"
		echo "sudo $up_cmd"
		echo
		echo "# Container images"
		echo "docker pull ghcr.io/$image_ns/levelrail:$tag"
		echo "docker pull docker.io/$hub_ns/levelrail:$tag"
		echo
		echo "# CLI only"
		echo "npx $npm_pkg@${tag#v}"
		echo '```'
		if [ "$pre" = "true" ]; then
			echo
			echo "This is a prerelease. Stable installs and the \`latest\` image tag do not move."
		fi
		echo
		echo "Per-registry digests are in the container images section."
	} | wrap polish-install
}

render_verify() {
	local base="https://github.com/$repo/releases/download/$tag"
	local wf="https://github.com/${repo//./\\.}/\\.github/workflows/release\\.yml@.*"
	{
		echo "## Provenance and signed checksums"
		echo
		echo "Every file is listed in [\`checksums.txt\`]($base/checksums.txt), signed keyless with cosign."
		echo
		echo '```bash'
		echo "curl -fsSLO $base/checksums.txt -O $base/checksums.txt.sigstore.json"
		echo "cosign verify-blob --bundle checksums.txt.sigstore.json \\"
		echo "  --certificate-identity-regexp '$wf' \\"
		echo "  --certificate-oidc-issuer https://token.actions.githubusercontent.com \\"
		echo "  checksums.txt"
		echo "sha256sum --check --ignore-missing checksums.txt"
		echo
		echo "# Build provenance of a downloaded file"
		echo "gh attestation verify levelrail-linux-amd64 --repo $repo"
		echo
		echo "# Container image signature"
		echo "cosign verify ghcr.io/$image_ns/levelrail:$tag \\"
		echo "  --certificate-identity-regexp '$wf' \\"
		echo "  --certificate-oidc-issuer https://token.actions.githubusercontent.com"
		echo '```'
		echo
		echo "Full guide: [Verify a release](https://github.com/$repo/blob/$tag/docs/verify-a-release.md). Attestations: https://github.com/$repo/attestations"
	} | wrap polish-verify
}

render_contributors() {
	local commits="$1" prev="$2" tmp="$work/contrib" total shown n=0 login mark
	[ -s "$commits" ] || return 0
	cut -f1 "$commits" | grep -v '^$' | grep -v -E "$bot_re" | sort | uniq -c | sort -k1,1nr -k2 >"$tmp"
	total="$(wc -l <"$tmp" | tr -d ' ')"
	[ "$total" -gt 0 ] || return 0
	{
		echo "## Contributors"
		echo
		echo "Thank you to everyone who shipped code in this release."
		echo
		while read -r _ login; do
			[ "$n" -lt "$max_contrib" ] || break
			n=$((n + 1))
			mark=""
			if [ -n "$prev" ] && ! has_earlier_commit "$login" "$prev"; then
				mark=" (first contribution)"
			fi
			echo "- @$login$mark"
		done <"$tmp"
		shown="$n"
		[ "$total" -gt "$shown" ] && echo "- +$((total - shown)) more"
	} | wrap polish-contributors
}

render_changelog() {
	local prev="$1"
	{
		if [ -n "$prev" ]; then
			echo "**Full changelog**: https://github.com/$repo/compare/$prev...$tag"
		else
			echo "**Full changelog**: https://github.com/$repo/commits/$tag"
		fi
	} | wrap polish-changelog
}

# ------------------------------------------------------------ assemble
# put <name> <blockfile> <where>: replace in place when the markers exist,
# otherwise insert at top, after the header block, or at the end.
put() {
	local name="$1" blk="$2" where="$3" cur="$work/cur" nxt="$work/nxt"
	cp "$work/body" "$cur"
	if grep -q "<!-- polish-$name:start -->" "$cur"; then
		awk -v s="<!-- polish-$name:start -->" -v e="<!-- polish-$name:end -->" -v bf="$blk" '
			index($0, s) { while ((getline l < bf) > 0) print l; skip = 1; next }
			skip { if (index($0, e)) skip = 0; next }
			{ print }' "$cur" >"$nxt"
	elif [ "$where" = top ]; then
		{ cat "$blk"; echo; cat "$cur"; } >"$nxt"
	elif [ "$where" = after-header ] && grep -q '<!-- polish-header:end -->' "$cur"; then
		awk -v e='<!-- polish-header:end -->' -v bf="$blk" '
			{ print }
			index($0, e) { print ""; while ((getline l < bf) > 0) print l }' "$cur" >"$nxt"
	else
		{ sed -e :a -e '/^\n*$/{$d;N;ba' -e '}' "$cur"; echo; cat "$blk"; } >"$nxt"
	fi
	mv "$nxt" "$work/body"
}

body_in="$(read_body)" || bail "cannot read release $tag"
printf '%s\n' "$body_in" >"$work/body"
cp "$work/body" "$work/original"
pre=false
is_prerelease && pre=true
prev="$(previous_tag "$pre")"
[ -n "$prev" ] || echo "polish-notes: no previous tag for $tag, treating as first release" >&2
read_commits "$prev" >"$work/commits" || warn "cannot read commit range"
sha=""
if [ -n "$fx" ]; then
	sha="$(cat "$fx/sha" 2>/dev/null || true)"
else
	sha="$(api "repos/$repo/commits/$tag" --jq .sha || true)"
fi
ref="$(asset_ref)"

# True when text outside our blocks already carries the section, so we add
# nothing the generator or a maintainer wrote.
provided() {
	awk '/^<!-- polish-.*:start -->/{s=1} !s{print} /^<!-- polish-.*:end -->/{s=0}' "$work/original" | grep -q -i -E "$1"
}

assemble() {
	local detail="$1" with_contrib="$2" f
	cp "$work/original" "$work/body"
	for f in header highlights install verify contributors changelog; do : >"$work/blk-$f"; done
	render_header "$ref" "$sha" >"$work/blk-header"
	render_highlights "$work/commits" "$detail" "$body_in" >"$work/blk-highlights"
	provided '^#+ install' || render_install "$pre" >"$work/blk-install"
	provided 'gh attestation verify' || render_verify >"$work/blk-verify"
	[ "$with_contrib" = yes ] && ! provided '^#+ contributors' && render_contributors "$work/commits" "$prev" >"$work/blk-contributors"
	provided 'full changelog' || render_changelog "$prev" >"$work/blk-changelog"
	put header "$work/blk-header" top
	[ -s "$work/blk-highlights" ] && put highlights "$work/blk-highlights" after-header
	[ -s "$work/blk-install" ] && put install "$work/blk-install" end
	[ -s "$work/blk-verify" ] && put verify "$work/blk-verify" end
	[ -s "$work/blk-contributors" ] && put contributors "$work/blk-contributors" end
	[ -s "$work/blk-changelog" ] && put changelog "$work/blk-changelog" end
}

# Over the size limit: drop highlight detail first, then the contributor list.
assemble full yes
if [ "$(wc -c <"$work/body")" -gt "$max_body" ]; then assemble short yes; fi
if [ "$(wc -c <"$work/body")" -gt "$max_body" ]; then assemble short no; fi
if [ "$(wc -c <"$work/body")" -gt "$max_body" ]; then
	bail "notes still over $max_body characters after trimming"
fi

if [ "$dry_run" = "--dry-run" ]; then
	cat "$work/body"
	exit 0
fi
if cmp -s "$work/body" "$work/original"; then
	echo "polish-notes: $tag already up to date"
	exit 0
fi
if gh release edit "$tag" --repo "$repo" --notes-file "$work/body" >/dev/null; then
	echo "polish-notes: updated $tag"
else
	warn "cannot edit release $tag, skipping"
fi
exit 0
