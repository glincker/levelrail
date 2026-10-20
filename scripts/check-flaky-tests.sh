#!/usr/bin/env bash
# Validates .github/flaky-tests.txt: well-formed entries, an issue link on
# each, and none older than QUARANTINE_MAX_DAYS (default 14). An expired
# entry is an error unless QUARANTINE_AGE=warn (the PR lane, so an overdue
# quarantine nags every PR without blocking unrelated work).
#
# Usage: scripts/check-flaky-tests.sh [file]
set -euo pipefail

file="${1:-.github/flaky-tests.txt}"
max_days="${QUARANTINE_MAX_DAYS:-14}"
[ -f "$file" ] || exit 0

to_epoch() {
	date -u -d "$1" +%s 2>/dev/null || date -u -j -f %Y-%m-%d "$1" +%s
}

now="$(date -u +%s)"
fail=0
n=0
while read -r pkg test issue since extra; do
	n=$((n + 1))
	case "$pkg" in '' | '#'*) continue ;; esac
	where="$file:$n"
	if [ -n "${extra:-}" ] || [ -z "${since:-}" ]; then
		echo "::error file=$file,line=$n::$where: want '<import path> <TestName> <issue URL> <YYYY-MM-DD>'"
		fail=1
		continue
	fi
	if ! [[ "$test" =~ ^(Test|Example|Fuzz)[A-Za-z0-9_]*$ ]]; then
		echo "::error file=$file,line=$n::$where: '$test' must be a top-level test name"
		fail=1
	fi
	if ! [[ "$issue" =~ ^https://github\.com/[^/]+/[^/]+/issues/[0-9]+$ ]]; then
		echo "::error file=$file,line=$n::$where: '$issue' is not a GitHub issue URL"
		fail=1
	fi
	if ! since_epoch="$(to_epoch "$since")"; then
		echo "::error file=$file,line=$n::$where: bad date '$since'"
		fail=1
		continue
	fi
	age=$(((now - since_epoch) / 86400))
	if [ "$age" -gt "$max_days" ]; then
		if [ "${QUARANTINE_AGE:-error}" = "warn" ]; then
			echo "::warning file=$file,line=$n::$test has been quarantined $age days (limit $max_days): fix or delete it ($issue)"
		else
			echo "::error file=$file,line=$n::$test has been quarantined $age days (limit $max_days): fix or delete it ($issue)"
			fail=1
		fi
	fi
done <"$file"

[ "$fail" -eq 0 ] && echo "flaky-tests.txt: ok"
exit "$fail"
