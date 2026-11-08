#!/usr/bin/env bash
# Helper for curated issue tracking: create an issue, add it to the org project, comment shipped shas.
set -euo pipefail

OWNER="glincker"
PROJECT=1

usage() {
  echo "usage: track.sh issue <title> <body-file> [--labels a,b] [--milestone vX] [--size S|M|L] [--surface API|CLI|Dashboard|Multi]" >&2
  echo "       track.sh ship <issue> <sha>" >&2
  exit 2
}

need_scopes() {
  if ! gh project field-list "$PROJECT" --owner "$OWNER" --format json >/dev/null 2>&1; then
    echo "gh cannot read the project. Run: gh auth refresh -s project,read:project" >&2
    exit 1
  fi
}

# Resolve "<field> <option>" to ids from live field data; nothing is cached.
option_ids() {
  gh project field-list "$PROJECT" --owner "$OWNER" --format json |
    jq -r --arg f "$1" --arg o "$2" '.fields[] | select(.name==$f) | [.id, (.options[]? | select(.name==$o) | .id)] | @tsv'
}

set_field() {
  local item="$1" proj="$2" field="$3" option="$4" ids fid oid
  ids=$(option_ids "$field" "$option")
  fid=${ids%%$'\t'*}
  oid=${ids#*$'\t'}
  if [ -z "$fid" ] || [ "$oid" = "$ids" ] || [ -z "$oid" ]; then
    echo "skip: no $field option named $option" >&2
    return 0
  fi
  gh project item-edit --id "$item" --project-id "$proj" --field-id "$fid" --single-select-option-id "$oid" >/dev/null
}

cmd_issue() {
  [ $# -ge 2 ] || usage
  local title="$1" body="$2" labels="" milestone="" size="" surface=""
  shift 2
  while [ $# -gt 0 ]; do
    case "$1" in
      --labels) labels="${2:-}"; shift 2 ;;
      --milestone) milestone="${2:-}"; shift 2 ;;
      --size) size="${2:-}"; shift 2 ;;
      --surface) surface="${2:-}"; shift 2 ;;
      *) usage ;;
    esac
  done
  need_scopes
  local args=(issue create --title "$title" --body-file "$body")
  [ -z "$labels" ] || args+=(--label "$labels")
  [ -z "$milestone" ] || args+=(--milestone "$milestone")
  local url item proj
  url=$(gh "${args[@]}")
  item=$(gh project item-add "$PROJECT" --owner "$OWNER" --url "$url" --format json | jq -r .id)
  proj=$(gh project view "$PROJECT" --owner "$OWNER" --format json | jq -r .id)
  [ -z "$size" ] || set_field "$item" "$proj" Size "$size"
  [ -z "$surface" ] || set_field "$item" "$proj" Surface "$surface"
  echo "$url"
}

cmd_ship() {
  [ $# -eq 2 ] || usage
  gh issue comment "$1" --body "Shipped in $2"
}

[ $# -ge 1 ] || usage
sub="$1"; shift
case "$sub" in
  issue) cmd_issue "$@" ;;
  ship) cmd_ship "$@" ;;
  *) usage ;;
esac
