#!/usr/bin/env bash
# Appends or replaces the image block in a GitHub release body.
# Usage: scripts/release/image-notes.sh <tag> [--dry-run]
# Env: GH_TOKEN, GITHUB_REPOSITORY, IMAGE_PRIMARY_TAG, IMAGE_TAGS (space separated),
#      CP_DIGEST, AGENT_DIGEST, CP_MIRRORED, AGENT_MIRRORED (true|false),
#      DOCKERHUB_NAMESPACE (default levelrail), IMAGE_NOTES_BODY_FILE (test only).
# Never fails the release: any gh error exits 0 with a warning.
set -uo pipefail

tag="${1:?usage: image-notes.sh <tag> [--dry-run]}"
dry_run="${2:-}"
repo="${GITHUB_REPOSITORY:-glincker/levelrail}"
primary="${IMAGE_PRIMARY_TAG:-$tag}"
ns="${DOCKERHUB_NAMESPACE:-levelrail}"
start='<!-- images:start -->'
end='<!-- images:end -->'

render() {
	local name digest mirrored
	echo "$start"
	echo "## Container images"
	echo
	# shellcheck disable=SC2016 # literal backticks for markdown
	echo "Tags: $(for t in ${IMAGE_TAGS:-$primary}; do printf '`%s` ' "$t"; done)"
	echo
	for name in levelrail levelrail-agent; do
		if [ "$name" = levelrail ]; then
			digest="${CP_DIGEST:-}"
			mirrored="${CP_MIRRORED:-false}"
		else
			digest="${AGENT_DIGEST:-}"
			mirrored="${AGENT_MIRRORED:-false}"
		fi
		echo "### $name"
		echo
		echo '```bash'
		echo "docker pull ghcr.io/glincker/$name:$primary"
		[ "$mirrored" = "true" ] && echo "docker pull docker.io/$ns/$name:$primary"
		echo '```'
		echo
		[ -n "$digest" ] && echo "- Digest: \`$digest\`"
		[ "$mirrored" = "true" ] && echo "- Docker Hub tags: https://hub.docker.com/r/$ns/$name/tags"
		echo
	done
	echo "$end"
}

merge() {
	# Drops any previous marked block, then appends the fresh one.
	awk -v s="$start" -v e="$end" '
		index($0, s) { skip = 1; next }
		skip && index($0, e) { skip = 0; next }
		!skip { print }
	' "$1" | sed -e :a -e '/^\n*$/{$d;N;ba' -e '}'
	echo
	render
}

body="$(mktemp)"
trap 'rm -f "$body"' EXIT
if [ -n "${IMAGE_NOTES_BODY_FILE:-}" ]; then
	cp "$IMAGE_NOTES_BODY_FILE" "$body"
elif ! gh release view "$tag" --repo "$repo" --json body --jq .body >"$body"; then
	echo "::warning::image-notes: cannot read release $tag, skipping"
	exit 0
fi

out="$(mktemp)"
trap 'rm -f "$body" "$out"' EXIT
merge "$body" >"$out"
if [ "$dry_run" = "--dry-run" ]; then
	cat "$out"
	exit 0
fi
if gh release edit "$tag" --repo "$repo" --notes-file "$out" >/dev/null; then
	echo "image-notes: updated $tag"
else
	echo "::warning::image-notes: cannot edit release $tag, skipping"
fi
exit 0
