#!/usr/bin/env bash
# Converts recorded .webm scenes to small GIFs: 800px wide, 12 fps, 128 colors.
set -euo pipefail
src="${1:?usage: to-gif.sh <webm_dir> <out_dir>}"
out="${2:?usage: to-gif.sh <webm_dir> <out_dir>}"
mkdir -p "$out"
for f in "$src"/*.webm; do
  n="$(basename "$f" .webm)"
  ffmpeg -v error -y -i "$f" \
    -vf "fps=12,scale=800:-1:flags=lanczos,split[a][b];[a]palettegen=max_colors=128[p];[b][p]paletteuse=dither=bayer:bayer_scale=4" \
    "$out/ui-$n.gif"
done
ls -l "$out"
