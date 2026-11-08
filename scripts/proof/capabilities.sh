#!/usr/bin/env bash
# Checks each line of capabilities.txt against the repo and prints a PASS or
# MISSING table. Exits non-zero on MISSING only with --strict.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$ROOT"
MANIFEST="${LEVELRAIL_PROOF_MANIFEST:-scripts/proof/capabilities.txt}"
strict=0
[ "${1:-}" = "--strict" ] && strict=1

cli=""
for c in ./levelrail-cli ./bin/levelrail-cli ./dist/levelrail-cli; do
  [ -x "$c" ] && cli="$c" && break
done

# Uses the built CLI help when present, else the shared command tree source.
check_cli() {
  local verb="$1" last="${1##* }"
  if [ -n "$cli" ]; then
    # shellcheck disable=SC2086
    if "$cli" $verb --help 2>&1 | grep -qi -- "$last"; then
      echo "PASS|$cli $verb --help"; else echo "MISSING|$cli $verb --help"; fi
  elif grep -qE "\"$last\":" internal/clitree/tree.go; then
    echo "PASS|grep \"$last\" internal/clitree/tree.go"
  else
    echo "MISSING|grep \"$last\" internal/clitree/tree.go"
  fi
}

echo "| Capability | Kind | Status | Check |"
echo "| --- | --- | --- | --- |"
missing=0
while IFS='|' read -r label kind check; do
  label="$(echo "$label" | xargs)"
  case "$label" in ''|'#'*) continue ;; esac
  kind="$(echo "$kind" | xargs)"
  check="$(echo "$check" | xargs)"
  case "$kind" in
    cli-verb) res="$(check_cli "$check")" ;;
    route)
      if grep -qF "\"$check\"" docs/public/openapi.json; then s=PASS; else s=MISSING; fi
      res="$s|grep -F '$check' docs/public/openapi.json" ;;
    setting)
      if grep -qxF "$check" internal/runtimesettings/keys.txt; then s=PASS; else s=MISSING; fi
      res="$s|grep -x $check internal/runtimesettings/keys.txt" ;;
    page)
      if [ -f "web/src/routes/$check" ]; then s=PASS; else s=MISSING; fi
      res="$s|test -f web/src/routes/$check" ;;
    doc)
      if [ -f "$check" ]; then s=PASS; else s=MISSING; fi
      res="$s|test -f $check" ;;
    *) res="MISSING|unknown kind $kind" ;;
  esac
  status="${res%%|*}"
  [ "$status" = "MISSING" ] && missing=$((missing + 1))
  # shellcheck disable=SC2016
  printf '| %s | %s | %s | `%s` |\n' "$label" "$kind" "$status" "${res#*|}"
done <"$MANIFEST"

echo
echo "Missing: $missing"
if [ "$strict" = 1 ] && [ "$missing" -gt 0 ]; then
  exit 1
fi
