#!/usr/bin/env bash
# Prints a markdown table of repo metrics. Every value comes from the command
# shown in the Source column; anything that cannot be measured says "not built".
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$ROOT"

# shellcheck disable=SC2016
row() { printf '| %s | %s | `%s` |\n' "$1" "$2" "$3"; }

size_of() {
  local f="$1"
  if [ -f "$f" ]; then
    printf '%s KB' "$(( $(wc -c <"$f" | tr -d ' ') / 1024 ))"
  else
    printf 'not built'
  fi
}

count_files() { find "$@" 2>/dev/null | wc -l | tr -d ' '; }

sha="$(git rev-parse --short=12 HEAD)"
date_c="$(git log -1 --format=%cI HEAD)"

echo "| Metric | Value | Source |"
echo "| --- | --- | --- |"
row "commit" "$sha" "git rev-parse --short=12 HEAD"
row "commit date" "$date_c" "git log -1 --format=%cI HEAD"

for b in levelrail levelrail-cli; do
  f=""
  for c in "./$b" "./bin/$b" "./dist/$b"; do
    [ -f "$c" ] && f="$c" && break
  done
  row "$b binary size" "$(size_of "${f:-./$b}")" "wc -c ${f:-./$b}"
done

if [ -d web/dist/assets ]; then
  js_kb="$(find web/dist/assets -maxdepth 1 -name 'index-*.js' -exec wc -c {} + | sort -n | tail -2 | head -1 | awk '{printf "%d", $1/1024}')"
  row "web main chunk (raw)" "${js_kb:-0} KB" "wc -c web/dist/assets/index-*.js (largest)"
else
  row "web main chunk (raw)" "not built" "wc -c web/dist/assets/index-*.js"
fi

verbs="$(grep -cE '^\s*"[a-z0-9-]+":' internal/clitree/tree.go || true)"
row "CLI verb entries" "$verbs" "grep -cE '^\\s*\"[a-z0-9-]+\":' internal/clitree/tree.go"
routes="$(grep -oE '^\s*"/[^"]+": *\{' docs/public/openapi.json | wc -l | tr -d ' ')"
row "API paths" "$routes" "grep -oE '\"/...\": {' docs/public/openapi.json"
row "runtime settings" "$(grep -c . internal/runtimesettings/keys.txt)" "grep -c . internal/runtimesettings/keys.txt"
mig="$(count_files internal/store/migrations -name '*.sql')"
top="$(find internal/store/migrations -name '*.sql' -exec basename {} \; | sed -E 's/^([0-9]+)_.*/\1/' | sort -n | tail -1)"
row "migrations" "$mig" "find internal/store/migrations -name '*.sql'"
row "highest migration" "$top" "ls internal/store/migrations | sort -n | tail -1"
row "Go test files" "$(count_files . -name '*_test.go' -not -path './web/*' -not -path '*/node_modules/*')" "find . -name '*_test.go'"
row "web test files" "$(count_files web/src \( -name '*.test.ts' -o -name '*.test.tsx' \))" "find web/src -name '*.test.ts*'"
row "ADRs" "$(count_files adr -name '[0-9]*.md')" "find adr -name '[0-9]*.md'"
row "docs pages" "$(count_files docs -maxdepth 1 \( -name '*.md' -o -name '*.mdx' \))" "find docs -maxdepth 1 -name '*.md*'"

if [ "${LEVELRAIL_PROOF_RSS:-0}" = "1" ]; then
  pid="$(pgrep -x levelrail | head -1 || true)"
  if [ -n "$pid" ]; then
    row "idle RSS" "$(( $(ps -o rss= -p "$pid" | tr -d ' ') / 1024 )) MB" "ps -o rss= -p $pid"
  else
    row "idle RSS" "not running" "pgrep -x levelrail"
  fi
fi
