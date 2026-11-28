#!/usr/bin/env bash
# Serves a signed dev announcement feed on http://127.0.0.1:${ANNOUNCE_DEV_PORT:-8765}/feed.v1.json
# for a dev instance (scripts/dev-local.sh). It signs with the public TEST keys
# of the announce golden vectors, so a release build never trusts it.
#
# Usage: scripts/announce-dev-feed.sh [--tamper | --replay]
#   (none)   one item per tier, sequence = now, valid signatures
#   --tamper serve a modified body with the old signature (instance must reject)
#   --replay serve sequence 1 (run a normal feed first, instance must reject)

set -euo pipefail

mode="ok"
case "${1:-}" in
	"") ;;
	--tamper) mode="tamper" ;;
	--replay) mode="replay" ;;
	*) echo "usage: $0 [--tamper|--replay]" >&2; exit 2 ;;
esac

repo_root="$(git rev-parse --show-toplevel)"
port="${ANNOUNCE_DEV_PORT:-8765}"
work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT
serve="$work/serve"
mkdir -p "$serve"

cd "$repo_root/tools"
go run ./announcedev -dir "$work"
seq="$(date +%s)"
[ "$mode" = "replay" ] && seq=1
go run ./announce build -dir "$work/items" -sequence "$seq" -out "$serve/feed.v1.json"
go run ./announce sign -feed "$serve/feed.v1.json" -roles "$work/roles.json" \
	-key "$work/keys/N.json" -key "$work/keys/S1.json" -key "$work/keys/S2.json"
if [ "$mode" = "tamper" ]; then
	sed 's/Dev critical notice/Dev critical notice (tampered)/' "$serve/feed.v1.json" >"$serve/tmp.json"
	mv "$serve/tmp.json" "$serve/feed.v1.json"
fi

keys="$(python3 -c 'import json,sys; print(",".join(json.load(open(sys.argv[1]))))' "$work/dev-keys.json")"
cat <<EOF

Mode: $mode, sequence $seq
Apply on the dev instance (needs APP_DEV_MODE=1, which scripts/dev-local.sh sets):
  updates.announcements            = all
  updates.announcements_feed_url   = http://127.0.0.1:$port/feed.v1.json
  updates.announcements_dev_keys   = $keys
Items target ">=0.0.1", so any tagged dev build matches. Serving, Ctrl-C to stop.

EOF
cd "$serve"
exec python3 -m http.server "$port" --bind 127.0.0.1
