#!/usr/bin/env bash
# Every top-level docs/*.md page needs a frontmatter `description:` so
# config.mts's transformHead has real per-page copy for og:description /
# twitter:description instead of silently falling back to the site-wide
# default. changelog/[slug].md is exempt: changelog.mts fills its
# description in per release at build time.
set -euo pipefail
cd "$(dirname "$0")/.."

missing=()
for f in docs/*.md; do
  if ! grep -q '^description:' "$f"; then
    missing+=("$f")
  fi
done

if [[ ${#missing[@]} -gt 0 ]]; then
  echo "Missing frontmatter description: in:"
  printf '  %s\n' "${missing[@]}"
  exit 1
fi

echo "All docs pages have a description tag."
