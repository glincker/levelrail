#!/usr/bin/env bash
# Verdict for the `CI required` job: reads the job's `needs` context as JSON
# from $NEEDS, prints PASS or FAIL with one reason line per failure, and exits
# non-zero on FAIL. Anything unexpected fails closed.
#
# Usage: NEEDS="$(cat needs.json)" scripts/ci-required-verdict.sh
set -uo pipefail

: "${NEEDS:?NEEDS must hold the needs context as JSON}"

fail=0
reasons=()
add_fail() {
	reasons+=("$1")
	echo "::error::$1"
	fail=1
}

if ! jq -e 'type == "object"' >/dev/null 2>&1 <<<"$NEEDS"; then
	add_fail "needs context is not a JSON object"
	NEEDS='{}'
fi

jq -r 'to_entries[] | "\(.key): \(.value.result)"' <<<"$NEEDS"

res() { jq -r --arg j "$1" '.[$j].result // "missing"' <<<"$NEEDS"; }
out() { jq -r --arg j "$1" --arg k "$2" '.[$j].outputs[$k] // ""' <<<"$NEEDS"; }

if [ "$(res changes)" != success ]; then
	add_fail "changes: change detection did not succeed"
fi

bad="$(jq -r 'to_entries[] | select(.value.result == "failure" or .value.result == "cancelled") | .key' <<<"$NEEDS" | paste -sd' ' -)"
if [ -n "$bad" ]; then
	add_fail "failed or cancelled: $bad"
fi

# A result outside success/skipped/failure/cancelled is a runner state we do
# not understand, so it must not pass.
odd="$(jq -r 'to_entries[] | select((.value.result // "missing") | IN("success", "skipped", "failure", "cancelled") | not) | .key' <<<"$NEEDS" | paste -sd' ' -)"
if [ -n "$odd" ]; then
	add_fail "unexpected result for: $odd"
fi

# Skipped means "area not affected" unless the PR is ready: then a skip of a
# job the planner says is needed must not pass. Drafts skip the heavy jobs on
# purpose, so they are exempt. CI_ON_DRAFTS is already folded into `draft`.
if [ "$(out changes draft)" != true ]; then
	expect() {
		if [ "$(res "$1")" = skipped ]; then
			add_fail "$1 was skipped but the planner expects it to run"
		fi
	}
	if [ "$(out changes go)" = true ] && [ "$(out plan has_tests)" = true ]; then expect test-go; fi
	if [ "$(out changes kit)" = true ]; then expect kit; fi
	if [ "$(out changes tfprovider)" = true ]; then expect tfprovider; fi
	if [ "$(out changes web)" = full ]; then
		case "$(out changes vitest)" in full | changed) expect web-test ;; esac
	fi
	if [ "$(out changes installer)" = true ]; then
		expect install-sh
		expect install-cli-macos
		expect install-cli-windows
	fi
fi

if [ "$fail" -ne 0 ]; then
	echo "FAIL"
	if [ -n "${GITHUB_STEP_SUMMARY:-}" ]; then
		{
			echo "### CI required: FAIL"
			printf -- '- %s\n' "${reasons[@]}"
		} >>"$GITHUB_STEP_SUMMARY"
	fi
	exit 1
fi
echo "PASS"
echo "all required jobs for this change passed"
[ -z "${GITHUB_STEP_SUMMARY:-}" ] || echo "### CI required: PASS" >>"$GITHUB_STEP_SUMMARY"
