#!/bin/bash
# Rewrites a GitHub release body with the generated notes (tools/releasenotes)
# and links an Announcements discussion if the release has none yet.
# Safe to re-run: only the block between the release-notes markers changes.
#
# Usage: scripts/release-notes.sh <tag> [--dry-run]
# Env: GH_TOKEN (falls back to `gh auth token`), GITHUB_REPOSITORY,
#      RELEASE_NOTES_CATEGORY (default: Announcements).
set -euo pipefail

tag="${1:?usage: release-notes.sh <tag> [--dry-run]}"
dry_run="${2:-}"
repo="${GITHUB_REPOSITORY:-glincker/levelrail}"
category="${RELEASE_NOTES_CATEGORY:-Announcements}"
root="$(cd "$(dirname "$0")/.." && pwd)"
binary="$(sed -n 's/^binary_name: *//p' "$root/brand.yaml")"
docs_url="$(sed -n 's/^docs_url: *//p' "$root/brand.yaml")"
owner="${repo%%/*}"
image_owner="$(printf %s "$owner" | tr "[:upper:]" "[:lower:]")"

if [ -z "${GH_TOKEN:-}" ]; then
	GH_TOKEN="$(gh auth token)"
	export GH_TOKEN
fi

notes="$(mktemp)"
trap 'rm -f "$notes"' EXIT

go -C "$root/tools" run ./releasenotes \
	-repo "$repo" -tag "$tag" -product "$binary" -docs-url "$docs_url" \
	-image "ghcr.io/${image_owner}/${binary}" \
	-image "ghcr.io/${image_owner}/${binary}-agent" \
	-out "$notes"

if [ "$dry_run" = "--dry-run" ]; then
	cat "$notes"
	exit 0
fi

gh release edit "$tag" --repo "$repo" --notes-file "$notes" >/dev/null
echo "release-notes: updated $tag"

existing="$(gh api "repos/$repo/releases/tags/$tag" --jq '.discussion_url // ""')"
if [ -n "$existing" ]; then
	echo "release-notes: discussion already linked: $existing"
	exit 0
fi
# shellcheck disable=SC2016 # GraphQL variables, not shell expansions
found="$(gh api graphql -f owner="$owner" -f name="${repo#*/}" -f query='
	query($owner: String!, $name: String!) {
		repository(owner: $owner, name: $name) {
			discussionCategories(first: 50) { nodes { name } }
		}
	}' --jq ".data.repository.discussionCategories.nodes[] | select(.name == \"$category\") | .name" 2>/dev/null || true)"
if [ -z "$found" ]; then
	echo "::warning::release-notes: discussion category '$category' not found, skipping announcement"
	exit 0
fi
gh release edit "$tag" --repo "$repo" --discussion-category "$category" >/dev/null
echo "release-notes: linked a '$category' discussion to $tag"
