#!/usr/bin/env bash
# Tests for render-comment.sh. Run: bash integrations/github-action/stack/render-comment.test.sh
set -euo pipefail

here="$(cd "$(dirname "$0")" && pwd)"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT
fail=0

check() {
  if ! grep -qF -- "$2" "$3"; then
    echo "FAIL $1: missing '$2'"
    fail=1
  fi
}

cat > "$tmp/changes.json" <<'JSON'
{"plan":{"summary":{"create":1,"update":2,"delete":0,"noop":3},"changes":[
{"action":"create","kind":"app","name":"web","fields":["image"]},
{"action":"noop","kind":"app","name":"old"}]},
"warnings":["slow"],"issues":[{"message":"bad ref"}]}
JSON
bash "$here/render-comment.sh" "$tmp/changes.json" shop abc123 > "$tmp/out1.md"
check marker '<!-- levelrail-stack:shop -->' "$tmp/out1.md"
check summary '**1** to add, **2** to change' "$tmp/out1.md"
check row '| create | `app/web` | 1 |' "$tmp/out1.md"
check issue '- bad ref' "$tmp/out1.md"
check hash 'Plan hash: `abc123`' "$tmp/out1.md"
if grep -qF 'app/old' "$tmp/out1.md"; then echo "FAIL noop row listed"; fail=1; fi

echo '{"plan":{"summary":{},"changes":[]}}' > "$tmp/empty.json"
bash "$here/render-comment.sh" "$tmp/empty.json" shop h > "$tmp/out2.md"
check nochanges 'No changes.' "$tmp/out2.md"

bash "$here/render-comment.sh" "$tmp/changes.json" shop abc123 200 > "$tmp/out3.md"
check truncnote 'Plan truncated' "$tmp/out3.md"
check truncmarker '<!-- levelrail-stack:shop -->' "$tmp/out3.md"
if [ "$(wc -c < "$tmp/out3.md")" -gt 260 ]; then echo "FAIL cap not applied"; fail=1; fi

[ "$fail" -eq 0 ] && echo "render-comment tests passed"
exit "$fail"
