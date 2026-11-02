#!/usr/bin/env bash
# Runs ./internal/store -short in the same shape CI uses, optionally with -race,
# and prints failures plus the slowest tests so a regression in speed is visible.
#
# Usage: scripts/gate-store.sh [-r name-regex] [-R]   (-R adds -race)
# Env: GATE_STORE_TIMEOUT (default 25m).
set -uo pipefail

cd "$(git rev-parse --show-toplevel)" || exit 2

timeout="${GATE_STORE_TIMEOUT:-25m}"
filter=""
race=""
while getopts "r:R" opt; do
	case "$opt" in
	r) filter="$OPTARG" ;;
	R) race="-race" ;;
	*) exit 2 ;;
	esac
done

log="$(mktemp -t levelrail-gate-store.XXXXXX)"
start="$(date +%s)"
args=(-short -json -timeout "$timeout")
[ -n "$race" ] && args+=("$race")
[ -n "$filter" ] && args+=(-run "$filter")

if go test "${args[@]}" ./internal/store >"$log" 2>&1; then
	status=0
else
	status=1
fi
elapsed=$(($(date +%s) - start))

python3 -I - "$log" <<'PY'
import json, sys
slow, fails = [], []
for line in open(sys.argv[1], errors="replace"):
    try:
        d = json.loads(line)
    except ValueError:
        print(line.rstrip())
        continue
    if d.get("Action") == "fail" and d.get("Test"):
        fails.append(d["Test"])
    if d.get("Action") == "pass" and d.get("Test") and "/" not in d["Test"]:
        slow.append((d.get("Elapsed", 0), d["Test"]))
for t in fails:
    print("FAILED:", t)
slow.sort(reverse=True)
for e, t in slow[:5]:
    print(f"slow: {e:7.1f}s {t}")
PY

if [ "$status" -eq 0 ]; then
	rm -f "$log"
	echo "gate-store: PASS in ${elapsed}s"
	exit 0
fi
echo "gate-store: FAIL in ${elapsed}s (log: $log)"
exit 1
