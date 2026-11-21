#!/usr/bin/env bash
# Prints one markdown bullet per failed job of a workflow run (name, link, failing step),
# so the "Release incomplete" issue names the real cause instead of only the asset check.
set -euo pipefail

run_id="${1:-}"
repo=""
[ -n "$run_id" ] || { echo "usage: failed-jobs.sh <run-id> [--repo owner/name]" >&2; exit 2; }
shift
while [ $# -gt 0 ]; do
	case "$1" in
	--repo) repo="${2:?--repo needs a value}"; shift 2 ;;
	*) echo "unknown argument: $1" >&2; exit 2 ;;
	esac
done

# gh fills the {owner}/{repo} placeholders from the current checkout when --repo is absent.
slug="$repo"
[ -n "$slug" ] || slug='{owner}/{repo}'
path="repos/${slug}/actions/runs/${run_id}/jobs?per_page=100"
gh api "$path" | jq -r '
	.jobs[] | select(.conclusion == "failure")
	| "- [\(.name)](\(.html_url)): failed at step \"\(([.steps[]? | select(.conclusion == "failure")][0].name) // "unknown")\""'
