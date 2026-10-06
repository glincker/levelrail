#!/usr/bin/env bash
# Every top-level docs/*.md page needs a frontmatter `description` (YAML or JSON style) so
# config.mts's transformHead has real per-page copy for og:description /
# twitter:description instead of silently falling back to the site-wide
# default. changelog/[slug].md is exempt: changelog.mts fills its
# description in per release at build time.
set -euo pipefail
cd "$(dirname "$0")/.."

missing=()
for f in docs/*.md; do
  if ! grep -q -E '^(description:|  "description":)' "$f"; then
    missing+=("$f")
  fi
done

if [[ ${#missing[@]} -gt 0 ]]; then
  echo "Missing frontmatter description: in:"
  printf '  %s\n' "${missing[@]}"
  exit 1
fi

# An unquoted YAML scalar containing ": " (or " #") does not parse and fails the
# whole VitePress build, so a description that exists is not enough.
unparseable=()
for f in docs/*.md; do
  line="$(grep -m1 -E '^description:' "$f" || true)"
  [[ -z "$line" ]] && continue
  value="${line#description:}"
  value="${value# }"
  case "$value" in \"* | \'*) continue ;; esac
  if [[ "$value" == *": "* || "$value" == *" #"* ]]; then
    unparseable+=("$f")
  fi
done

if [[ ${#unparseable[@]} -gt 0 ]]; then
  echo "Unquoted description contains \": \" or \" #\" (invalid YAML), quote it or reword in:"
  printf '  %s\n' "${unparseable[@]}"
  exit 1
fi

echo "All docs pages have a description tag."
