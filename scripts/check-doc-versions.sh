#!/usr/bin/env bash
# Fails when a named doc shows a release version older than the top CHANGELOG entry,
# so install and rollback examples do not go stale. Use vX.Y.Z as a placeholder.
# Usage: scripts/check-doc-versions.sh [file ...]
set -u
cd "$(dirname "$0")/.." || exit 2

top="$(sed -n 's/^## \[\([0-9][0-9]*\.[0-9][0-9]*\.[0-9][0-9]*\).*/\1/p' CHANGELOG.md | head -1)"
if [ -z "$top" ]; then
  echo "check-doc-versions: no version found at the top of CHANGELOG.md" >&2
  exit 2
fi

if [ "$#" -gt 0 ]; then
  files=("$@")
else
  files=(docs/getting-started.md docs/rollback.md docs/first-15-minutes.md)
fi

python3 -I - "$top" "${files[@]}" <<'PY'
import re, sys

def key(v):
    return tuple(int(x) for x in v.split("."))

top, files = sys.argv[1], sys.argv[2:]
bad = 0
for f in files:
    for n, line in enumerate(open(f, encoding="utf-8"), 1):
        # A leading v and no fourth dotted number, so IPs and API versions are skipped.
        for m in re.finditer(r"(?<![\w.])v(\d+\.\d+\.\d+)(?![\d.]*\d)", line):
            if key(m.group(1)) < key(top):
                print(f"{f}:{n}: v{m.group(1)} is older than the top CHANGELOG version {top}; use vX.Y.Z")
                bad = 1
sys.exit(bad)
PY
