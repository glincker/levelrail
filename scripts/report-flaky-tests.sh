#!/usr/bin/env bash
# Opens, or comments on, one `flaky-test` issue per flaky test found by
# scripts/ci-go-test.sh (its flaky.txt files), so flakes absorbed by reruns
# still leave a tracked trail.
#
# Usage: scripts/report-flaky-tests.sh <flaky.txt>...
# Needs gh authenticated with issues: write, and GITHUB_SERVER_URL,
# GITHUB_REPOSITORY, GITHUB_RUN_ID for the run link.
set -euo pipefail

run_url="${GITHUB_SERVER_URL:-https://github.com}/${GITHUB_REPOSITORY:?}/actions/runs/${GITHUB_RUN_ID:?}"
label="flaky-test"

gh label create "$label" --color d93f0b --description "Test that failed and passed on the same commit" 2>/dev/null || true

cat "$@" 2>/dev/null | sort -u | while read -r pkg test attempts; do
	[ -n "$test" ] || continue
	short="${pkg#github.com/GLINCKER/levelrail/}"
	title="Flaky test: $short $test"
	body="\`$test\` in \`$short\` failed $attempts attempts and passed otherwise in $run_url (workflow: ${GITHUB_WORKFLOW:-unknown}, ref: ${GITHUB_REF_NAME:-unknown}, sha: ${GITHUB_SHA:-unknown})."
	existing="$(gh issue list --label "$label" --state open --search "\"$title\" in:title" --json number,title \
		--jq ".[] | select(.title == \"$title\") | .number" | head -n1)"
	if [ -n "$existing" ]; then
		gh issue comment "$existing" --body "Flaked again: $body"
		echo "commented on #$existing: $title"
	else
		gh issue create --title "$title" --label "$label" --label "type/test" --body "$body

Quarantine only with an issue link in \`.github/flaky-tests.txt\`; fix or delete within 14 days. See CONTRIBUTING.md#flaky-tests."
		echo "opened: $title"
	fi
done
