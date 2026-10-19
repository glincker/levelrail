#!/usr/bin/env bash
# Deploys catalog templates in a local Docker harness and writes a pass report.
# Usage: scripts/catalog-smoke.sh [-n TOP] [-i id1,id2] [-t SECONDS] [-k] [-r]
#   -n TOP      how many templates (default 50)
#   -i IDS      only these template IDs
#   -t SECONDS  per-template wait for healthy (default 180)
#   -k          keep images pulled by the run (default: remove images the run pulled)
#   -r          also rewrite docs/templates-status.md from the results
# Every project is named lrsmoke-<id>; containers, volumes and networks are removed
# after each template and again on exit or interrupt.
set -u

top=50
ids=""
wait_s=180
keep_images=0
write_docs=0
while getopts "n:i:t:kr" opt; do
  case "$opt" in
    n) top="$OPTARG" ;;
    i) ids="$OPTARG" ;;
    t) wait_s="$OPTARG" ;;
    k) keep_images=1 ;;
    r) write_docs=1 ;;
    *) exit 2 ;;
  esac
done

repo="$(cd "$(dirname "$0")/.." && pwd)"
work="$(mktemp -d "${TMPDIR:-/tmp}/catalog-smoke.XXXXXX")"
results="$work/results.tsv"
: > "$results"

if ! docker info >/dev/null 2>&1; then
  echo "docker is not running" >&2
  exit 1
fi

images_before="$work/images.before"
docker image ls -q --no-trunc | sort -u > "$images_before"

sweep() {
  local ids_
  ids_="$(docker ps -a --filter "label=com.docker.compose.project" --format '{{.ID}} {{.Label "com.docker.compose.project"}}' 2>/dev/null | awk '$2 ~ /^lrsmoke-/ {print $1}')"
  [ -n "$ids_" ] && docker rm -f -v $ids_ >/dev/null 2>&1
  local vols
  vols="$(docker volume ls --filter "label=com.docker.compose.project" --format '{{.Name}} {{.Label "com.docker.compose.project"}}' 2>/dev/null | awk '$2 ~ /^lrsmoke-/ {print $1}')"
  [ -n "$vols" ] && docker volume rm -f $vols >/dev/null 2>&1
  local nets
  nets="$(docker network ls --filter "label=com.docker.compose.project" --format '{{.ID}} {{.Label "com.docker.compose.project"}}' 2>/dev/null | awk '$2 ~ /^lrsmoke-/ {print $1}')"
  [ -n "$nets" ] && docker network rm $nets >/dev/null 2>&1
  return 0
}

cleanup() {
  sweep
  if [ "$keep_images" -eq 0 ]; then
    local new_images
    new_images="$(docker image ls -q --no-trunc | sort -u | comm -13 "$images_before" -)"
    [ -n "$new_images" ] && docker image rm -f $new_images >/dev/null 2>&1
  fi
  rm -rf "$work/templates"
}
trap cleanup EXIT
trap 'exit 130' INT TERM

prep_args=(prepare -top "$top" -out "$work/templates")
[ -n "$ids" ] && prep_args+=(-ids "$ids")
(cd "$repo" && go run ./scripts/catalog-smoke "${prep_args[@]}") || exit 1

while IFS=$'\t' read -r id name file; do
  proj="lrsmoke-$(echo "$id" | tr -c 'a-z0-9\n' '-' | tr -s '-')"
  start=$(date +%s)
  log="$work/$id.log"
  status=pass
  note=""
  if ! docker compose -p "$proj" -f "$file" pull --quiet >"$log" 2>&1; then
    status=pull_failed
    note="$(tail -n 1 "$log" | cut -c1-140)"
  elif ! docker compose -p "$proj" -f "$file" up -d --wait --wait-timeout "$wait_s" >>"$log" 2>&1; then
    status=fail
    note="$(grep -iE 'unhealthy|exited|error|timed out' "$log" | tail -n 1 | cut -c1-140)"
  else
    bad="$(docker compose -p "$proj" -f "$file" ps -a --format '{{.Service}} {{.State}}' | awk '$2 == "exited" || $2 == "dead" {print $1}' | tr '\n' ' ')"
    if [ -n "$bad" ]; then
      status=fail
      note="exited: $bad"
    fi
  fi
  secs=$(( $(date +%s) - start ))
  printf '%s\t%s\t%s\t%s\n' "$id" "$status" "$secs" "$note" >> "$results"
  printf '%-4s %-28s %ss %s\n' "$status" "$id" "$secs" "$note"
  docker compose -p "$proj" -f "$file" down -v --remove-orphans >/dev/null 2>&1
done < "$work/templates/index.tsv"

if [ -s "$work/templates/skipped.tsv" ]; then
  echo "skipped (need configuration or unparseable):"
  sed 's/^/  /' "$work/templates/skipped.tsv"
fi

total=$(wc -l < "$results" | tr -d ' ')
passed=$(awk -F'\t' '$2 == "pass"' "$results" | wc -l | tr -d ' ')
echo "pass rate: $passed of $total"

echo "results: $results"

if [ "$write_docs" -eq 1 ]; then
  (cd "$repo" && go run ./scripts/catalog-smoke report -results "$results" -out docs/templates-status.md)
fi
