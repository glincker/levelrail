#!/usr/bin/env bash
# Diff-driven dashboard screenshots. Selects pages from affected.map that match
# the PR's changed files, runs a throwaway local instance, captures at most
# LEVELRAIL_PROOF_MAX_SHOTS pages in one browser, writes <label>.webp files
# plus shots.tsv (file, label, note) into $PROOF_OUT/shots.
# Never builds (use run.sh --build), never fakes an image.
set -euo pipefail
# shellcheck source=lib.sh
. "$(dirname "${BASH_SOURCE[0]}")/lib.sh"

MAX_SHOTS="${LEVELRAIL_PROOF_MAX_SHOTS:-4}"
MAX_KB="${LEVELRAIL_PROOF_MAX_KB:-60}"
SHOTS_TIMEOUT="${LEVELRAIL_PROOF_SHOTS_TIMEOUT:-90}"
OUT="$PROOF_OUT/shots"
SHOT_DIR="$PROOF_ROOT/scripts/screenshots"

files="$(changed_files)"
sel=()
seen=" "
while IFS='|' read -r globs route seed label; do
  globs="$(echo "$globs" | xargs)"
  case "$globs" in ''|'#'*) continue ;; esac
  glob_hits "$globs" "$files" || continue
  route="$(echo "$route" | xargs)"; seed="$(echo "$seed" | xargs)"
  label="$(echo "$label" | sed -E 's/^[[:space:]]+|[[:space:]]+$//g')"
  case "$seen" in *" $route "*) continue ;; esac
  seen="$seen$route "
  sel+=("$route|$seed|$label")
  [ "${#sel[@]}" -ge "$MAX_SHOTS" ] && break
done <"$PROOF_ROOT/scripts/proof/affected.map"

if [ "${#sel[@]}" -eq 0 ]; then
  echo "no mapped pages for this diff"
  exit 0
fi

bin="$(find_bin levelrail || true)"
cli="$(find_bin levelrail-cli || true)"
[ -n "$bin" ] || { echo "shots: levelrail binary not found, build it first (run.sh --build or CI)" >&2; exit 3; }
[ -n "$cli" ] || { echo "shots: levelrail-cli binary not found" >&2; exit 3; }
command -v shot-scraper >/dev/null 2>&1 || { echo "shots: shot-scraper not installed (pip install shot-scraper && shot-scraper install)" >&2; exit 3; }
command -v cwebp >/dev/null 2>&1 || { echo "shots: cwebp not installed" >&2; exit 3; }

scratch="$(mktemp -d "${TMPDIR:-/tmp}/levelrail-proof.XXXXXX")"
srv_pid=""
cleanup() {
  [ -n "$srv_pid" ] && kill "$srv_pid" 2>/dev/null || true
  [ -n "$srv_pid" ] && wait "$srv_pid" 2>/dev/null || true
  rm -rf "$scratch"
}
trap cleanup EXIT

port=$((20000 + RANDOM % 20000))
base="http://127.0.0.1:$port"
mkdir -p "$scratch/data" "$scratch/home" "$OUT"
rm -f "$OUT"/*.webp "$OUT/shots.tsv"

# Throwaway dev-mode instance: temp data dir, loopback only, no Docker needed.
(cd "$PROOF_ROOT" && APP_DEV_MODE=1 APP_DATA_DIR="$scratch/data" APP_HTTP_ADDR="127.0.0.1:$port" \
  APP_AGENT_ADDR="127.0.0.1:0" APP_INGRESS_HTTP_ADDR="127.0.0.1:0" APP_INGRESS_HTTPS_ADDR="127.0.0.1:0" \
  lowprio "$bin" >"$scratch/server.log" 2>&1) &
srv_pid=$!

ready=0
for _ in $(seq 1 40); do
  curl -fsS -o /dev/null "$base/api/v1/brand" 2>/dev/null && { ready=1; break; }
  kill -0 "$srv_pid" 2>/dev/null || break
  sleep 1
done
[ "$ready" = 1 ] || { echo "shots: local instance did not start (no image was faked)" >&2; exit 3; }

LEVELRAIL_URL="$base" tmo 30 nice -n 15 python3 "$SHOT_DIR/login_state.py" "$scratch/state.json" \
  || { echo "shots: browser login failed (needs python3 with playwright)" >&2; exit 3; }

yaml="$scratch/shots.yml"
: >"$yaml"
note_lines=()
for item in "${sel[@]}"; do
  IFS='|' read -r route seed label <<<"$item"
  app="proof-app"; note="seeded"
  if [ "$seed" = "none" ] || [ ! -x "$PROOF_ROOT/scripts/proof/seed/$seed.sh" ]; then
    note="needs real data"
  else
    if ! LEVELRAIL_PROOF_CLI="$cli" HOME="$scratch/home" LEVELRAIL_URL="$base" \
      tmo 30 nice -n 15 "$PROOF_ROOT/scripts/proof/seed/$seed.sh" >/dev/null 2>&1; then
      note="needs real data"
    fi
  fi
  slug="$(echo "$label" | tr 'A-Z ' 'a-z-' | tr -cd 'a-z0-9-')"
  {
    echo "- output: $scratch/$slug.png"
    echo "  url: $base${route//\{app\}/$app}"
    echo "  width: 1280"
    echo "  height: 800"
    echo "  wait: 2000"
  } >>"$yaml"
  note_lines+=("$slug|$label|$note")
done

# One browser run, sequential, no video or tracing, default scale (no retina).
(cd "$PROOF_ROOT" && tmo "$SHOTS_TIMEOUT" nice -n 15 shot-scraper multi "$yaml" --auth "$scratch/state.json" --timeout 30000) \
  || { echo "shots: browser capture failed or timed out" >&2; exit 3; }

for nl in "${note_lines[@]}"; do
  IFS='|' read -r slug label note <<<"$nl"
  png="$scratch/$slug.png"
  [ -f "$png" ] || continue
  for q in 70 60 50 40 30; do
    lowprio cwebp -quiet -q "$q" "$png" -o "$OUT/$slug.webp"
    [ "$(( $(wc -c <"$OUT/$slug.webp") / 1024 ))" -le "$MAX_KB" ] && break
  done
  printf '%s.webp\t%s\t%s\n' "$slug" "$label" "$note" >>"$OUT/shots.tsv"
done
echo "shots: wrote $(wc -l <"$OUT/shots.tsv" | tr -d ' ') image(s) to $OUT"
