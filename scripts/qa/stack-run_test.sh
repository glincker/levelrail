#!/usr/bin/env bash
# Dry-run tests for stack-run.sh. levelrail-cli, curl and doctl are stubs that
# fail loudly, so a test can never touch a real server.
set -euo pipefail

here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
tmp="$(mktemp -d)"
trap 'find "$tmp" -mindepth 1 -delete; rmdir "$tmp"' EXIT

mkdir "$tmp/bin"
for tool in levelrail-cli curl doctl; do
	printf '#!/usr/bin/env bash\necho "stub %s called: $*" >&2\nexit 99\n' "$tool" >"$tmp/bin/$tool"
	chmod +x "$tmp/bin/$tool"
done
export PATH="$tmp/bin:$PATH"

pass=0
fail=0
contains() {
	if [[ "$2" == *"$3"* ]]; then
		pass=$((pass + 1))
	else
		fail=$((fail + 1))
		printf 'FAIL %s: missing %s in:\n%s\n' "$1" "$3" "$2"
	fi
}
lacks() {
	if [[ "$2" == *"$3"* ]]; then
		fail=$((fail + 1))
		printf 'FAIL %s: unexpected %s in:\n%s\n' "$1" "$3" "$2"
	else
		pass=$((pass + 1))
	fi
}

for s in hello-web app-db rollback-loop; do
	out="$("$here/stack-run.sh" "$s" --dry-run --server http://10.0.0.5:8080 --token t0k 2>&1)"
	contains "$s apply" "$out" "levelrail-cli stack apply qa-$s-"
	contains "$s wait" "$out" "apps deploys wait"
	contains "$s destroy" "$out" "stack delete qa-$s-"
	contains "$s pass" "$out" "PASS $s qa-$s-"
	lacks "$s no token leak" "$out" "t0k"
	lacks "$s no stub" "$out" "stub "
done

out="$("$here/stack-run.sh" rollback-loop --dry-run 2>&1)"
contains rollback_cmd "$out" "stack rollback"
contains rollback_loop "$out" "request loop"

out="$("$here/stack-run.sh" hello-web --dry-run --keep 2>&1)"
contains keep "$out" "keeping stack"
lacks keep_no_delete "$out" "stack delete"

if "$here/stack-run.sh" nope --dry-run >/dev/null 2>&1; then
	fail=$((fail + 1))
	echo "FAIL unknown scenario accepted"
else
	pass=$((pass + 1))
fi

for f in "$here"/stacks/*.yaml; do
	contains "template $(basename "$f")" "$(cat "$f")" "@SUFFIX@"
done
contains from_ref "$(cat "$here/stacks/app-db.yaml")" "from: { database: qa-db-@SUFFIX@, field: url }"

echo "stack-run_test: $pass passed, $fail failed"
[ "$fail" = 0 ]
