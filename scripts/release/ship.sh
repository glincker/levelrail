#!/usr/bin/env bash
# Reports release readiness for one version. It never merges or dispatches anything.
# Usage: scripts/release/ship.sh <version> [--after-merge] [--repo owner/name]
set -euo pipefail
root="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$root"

version="${1:?usage: ship.sh <version> [--after-merge] [--repo owner/name]}"
shift
after=false
repo_args=()
while [ $# -gt 0 ]; do
	case "$1" in
	--after-merge) after=true; shift ;;
	--repo) repo_args=(--repo "${2:?--repo needs a value}"); shift 2 ;;
	*) echo "unknown argument: $1" >&2; exit 2 ;;
	esac
done
tag="v${version#v}"
version="${version#v}"
wait_secs="${SHIP_WAIT_SECS:-1800}"

if [ "$after" = "false" ]; then
	pr="$(gh pr list "${repo_args[@]}" --base main --head "release/v${version}" --state open --json number --jq '.[0].number // empty')"
	[ -n "$pr" ] || { echo "no open release PR from release/v${version} into main" >&2; exit 1; }
	echo "release PR: #${pr}"
	if gh pr checks "$pr" "${repo_args[@]}" --required >/dev/null 2>&1; then
		echo "required checks: green"
	else
		echo "required checks: NOT green"
		gh pr checks "$pr" "${repo_args[@]}" --required 2>&1 | grep -Ev 'pass|skipping' | head -10 || true
		exit 1
	fi
	echo "release gate: runs inside release.yml; rehearse locally with scripts/release-gate.sh --help"
	echo
	scripts/release-squash-message.sh "$version"
	exit 0
fi

# Bounded wait: poll for the tag once a minute, then watch the run, never longer than the cap.
deadline=$((SECONDS + wait_secs))
until gh release view "$tag" "${repo_args[@]}" >/dev/null 2>&1; do
	[ $SECONDS -lt $deadline ] || { echo "tag ${tag} did not appear within ${wait_secs}s" >&2; exit 1; }
	sleep 60
done
run_id="$(gh run list "${repo_args[@]}" --workflow release.yml --limit 5 --json databaseId,headBranch --jq "[.[] | select(.headBranch == \"${tag}\")][0].databaseId // empty")"
if [ -n "$run_id" ]; then
	timeout "$wait_secs" gh run watch "$run_id" "${repo_args[@]}" --exit-status >/dev/null 2>&1 || echo "Release run ${run_id} did not finish green"
fi
if scripts/release/verify-assets.sh "$tag" "${repo_args[@]}"; then
	exit 0
fi
echo "redispatch (run by hand): gh workflow run release.yml --ref main -f tag=${tag}"
exit 1
