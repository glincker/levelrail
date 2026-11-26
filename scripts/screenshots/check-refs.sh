#!/usr/bin/env bash
# Fails when a doc points at a missing screenshot or a screenshot is unreferenced.
# README, docs, the README carousel sources and scripts/screenshots/site-names.txt count as references.
set -uo pipefail
cd "$(dirname "${BASH_SOURCE[0]}")/../.."
dir=docs/assets/screenshots
status=0

refs="$(grep -rhoE 'assets/screenshots/[a-z0-9-]+\.png' README.md docs 2>/dev/null | sed 's#.*/##' | sort -u)"
carousel="$(ls docs/assets/readme 2>/dev/null | sed -nE 's/^thumb-[0-9]+-(.*)-(dark|light)\.webp$/\1.png/p' | sort -u)"

norm="$(printf '%s\n' "$refs" | sed 's/-light\.png$/.png/' | sort -u)"

site="$(grep -v '^#' scripts/screenshots/site-names.txt 2>/dev/null | sed 's/$/.png/')"

for f in $refs; do
  [ -f "$dir/$f" ] || { echo "missing file for reference: $dir/$f"; status=1; }
done

for f in $(ls "$dir" | grep '\.png$'); do
  base="${f%-light.png}"
  [ "$base" = "$f" ] && base="${f%.png}"
  name="$base.png"
  if ! printf '%s\n%s\n%s\n' "$norm" "$carousel" "$site" | grep -qx "$name"; then
    echo "unreferenced screenshot: $dir/$f"; status=1
  fi
  [ -f "$dir/$base.png" ] && [ -f "$dir/$base-light.png" ] || { echo "dark/light pair incomplete: $base"; status=1; }
done

[ "$status" -eq 0 ] && echo "check-refs: ok"
exit "$status"
