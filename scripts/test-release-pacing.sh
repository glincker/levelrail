#!/usr/bin/env bash
# Self test for release-pacing.sh against a throwaway repo.
set -euo pipefail
script="$(cd "$(dirname "$0")" && pwd)/release-pacing.sh"
d="$(mktemp -d)"; trap 'rm -rf "$d"' EXIT
cd "$d"
env -u GIT_DIR -u GIT_WORK_TREE git init -q .
git -c user.name=t -c user.email=t@t commit -q --allow-empty -m x
git tag v0.7.0
t0="$(git log -1 --format=%ct)"

fails=0
check() { # name want-exit title labels offset-seconds
  local name="$1" want="$2" title="$3" labels="$4" off="$5" got=0
  PR_TITLE="$title" PR_LABELS="$labels" NOW=$(( t0 + off )) bash "$script" >/dev/null 2>&1 || got=$?
  if [ "$got" != "$want" ]; then echo "FAIL $name: exit $got want $want"; fails=1; else echo "ok   $name"; fi
}
check "patch after cooldown"        0 "chore(main): release 0.7.1" ""                   43200
check "patch inside cooldown"       1 "chore(main): release 0.7.1" ""                   600
check "cooldown override"           0 "chore(main): release 0.7.1" "skip-release-cooldown" 600
check "minor jump blocked"          1 "chore(main): release 0.8.0" ""                   43200
check "minor jump approved"         0 "chore(main): release 0.8.0" "allow-minor-release" 43200
check "non release title ignored"   0 "feat: something"            ""                   0
exit "$fails"
