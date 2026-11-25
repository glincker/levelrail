#!/usr/bin/env bash
# Usage: PROOF_HOST=user@host PROOF_CONFIRM=yes [PROOF_LIVE=1] scripts/host-proof/run.sh <item>
# Without PROOF_LIVE=1 nothing runs: checks are printed only.
set -euo pipefail
here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=lib.sh
. "$here/lib.sh"

[ "$#" -eq 1 ] || proof_die "usage: run.sh <item>"
item="$1"
if ! printf '%s' "$item" | grep -Eq '^[a-z0-9][a-z0-9-]*$'; then
	proof_die "invalid item name: $item"
fi
proof_gate

file="$here/items/$item.sh"
[ -f "$file" ] || proof_die "no such item: $item"
fn="prove_${item//-/_}"
# shellcheck source=/dev/null
. "$file"
declare -F "$fn" >/dev/null || proof_die "$file does not define $fn"

"$fn"

printf '\nsummary: pass=%s fail=%s dry=%s\n' "$PROOF_PASSES" "$PROOF_FAILS" "$PROOF_DRY"
if [ "$PROOF_FAILS" -gt 0 ]; then
	exit 1
fi
if [ "${PROOF_LIVE:-}" != "1" ]; then
	echo "DRY RUN: nothing executed, this is not a proof"
fi
exit 0
