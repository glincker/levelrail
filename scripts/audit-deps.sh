#!/usr/bin/env bash
# Dependency audit report: govulncheck for Go (if installed) and npm audit for web/.
# Never installs anything. Exit 0 when clean or skipped, 1 when a tool reports findings.
# Set AUDIT_OFFLINE=1 to make npm use only its local cache.
set -u

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
status=0

echo "== Go (govulncheck)"
if command -v govulncheck >/dev/null 2>&1; then
  (cd "$root" && govulncheck ./...) || status=1
else
  echo "govulncheck not installed, skipped (go install golang.org/x/vuln/cmd/govulncheck@latest)"
fi

echo "== Web (npm audit)"
if [ ! -f "$root/web/package-lock.json" ]; then
  echo "web/package-lock.json missing, skipped"
elif command -v npm >/dev/null 2>&1; then
  offline=()
  [ "${AUDIT_OFFLINE:-0}" = "1" ] && offline=(--offline)
  (cd "$root/web" && npm audit --omit=dev --audit-level=high ${offline[@]+"${offline[@]}"}) || status=1
else
  echo "npm not installed, skipped"
fi

exit "$status"
