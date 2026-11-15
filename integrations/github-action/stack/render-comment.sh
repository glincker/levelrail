#!/usr/bin/env bash
# Renders the sticky plan comment from `stack plan --json` output.
# Usage: render-comment.sh PLAN_JSON STACK HASH [MAX_CHARS]
set -euo pipefail

plan_file="${1:?plan json path}"
stack="${2:?stack name}"
hash="${3:-}"
max="${4:-${LR_COMMENT_MAX_CHARS:-60000}}"

# A non numeric cap would break the arithmetic below, so fall back to the default.
case "$max" in '' | *[!0-9]*) max=60000 ;; esac

body=$(
  echo "<!-- levelrail-stack:${stack} -->"
  echo "### Levelrail stack plan: \`${stack}\`"
  echo
  jq -r '
    (.plan.summary // {}) as $s
    | "**\($s.create // 0)** to add, **\($s.update // 0)** to change, **\($s.delete // 0)** to remove, \($s.noop // 0) unchanged"
      + (if ($s.error // 0) > 0 then ", **\($s.error)** with errors" else "" end),
      "",
      (if (.pinned_revision // 0) > 0 then "Pinned to revision \(.pinned_revision): applies are refused until the pin is removed.\n" else "" end),
      ([.plan.changes[]? | select(.action != "noop")] as $c
        | if ($c | length) == 0 then "No changes."
          else "| Action | Resource | Fields |\n| --- | --- | --- |\n"
            + ($c | map("| \(.action) | `\(.kind)/\(.name)` | \((.fields // []) | length) |") | join("\n"))
          end),
      "",
      (if ((.issues // []) | length) > 0 then "Issues:\n" + ((.issues // []) | map("- \(.message // tostring)") | join("\n")) + "\n" else "" end),
      (if ((.warnings // []) | length) > 0 then "Warnings:\n" + ((.warnings // []) | map("- \(.)") | join("\n")) + "\n" else "" end)
  ' "$plan_file"
  echo "Plan hash: \`${hash}\`"
)

# The marker line stays at the top so the next run still finds and updates the comment.
if [ "${#body}" -gt "$max" ]; then
  note=$'\n\n_Plan truncated, see the job summary for the full text._\n'
  keep=$((max - ${#note}))
  [ "$keep" -gt 0 ] || keep=0
  body="${body:0:$keep}${note}"
fi
printf '%s\n' "$body"
