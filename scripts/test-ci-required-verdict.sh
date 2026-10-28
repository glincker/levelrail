#!/usr/bin/env bash
# Table tests for scripts/ci-required-verdict.sh (the `CI required` gate): a
# skipped heavy lane passes on drafts only, anything unexpected fails closed.
# Overrides are `job=result` or `job.output=value`; the base is a ready PR
# where every needed job succeeded and nothing is affected.
#
# Usage: scripts/test-ci-required-verdict.sh
set -u

cd "$(git rev-parse --show-toplevel)" || exit 2

fail=0
checks=0

jobs="changes plan build-vet lint kit tfprovider test-go test-api test-rest web-check web-test install-sh install-cli-macos install-cli-windows release-pacing"

needs_json() {
	local spec="$*" job doc='{}' kv key val
	for job in $jobs; do
		doc="$(jq -c --arg j "$job" '.[$j] = {result: "success", outputs: {}}' <<<"$doc")"
	done
	doc="$(jq -c '.changes.outputs = {go:"false",web:"none",vitest:"none",kit:"false",tfprovider:"false",installer:"false",draft:"false"} | .plan.outputs = {has_tests:"false"} | ."release-pacing".result = "skipped"' <<<"$doc")"
	for kv in $spec; do
		key="${kv%%=*}"
		val="${kv#*=}"
		if [[ "$key" == *.* ]]; then
			doc="$(jq -c --arg j "${key%%.*}" --arg k "${key#*.}" --arg v "$val" '.[$j].outputs[$k] = $v' <<<"$doc")"
		elif [ "$val" = DROP ]; then
			doc="$(jq -c --arg j "$key" 'del(.[$j])' <<<"$doc")"
		else
			doc="$(jq -c --arg j "$key" --arg v "$val" '.[$j].result = $v' <<<"$doc")"
		fi
	done
	printf '%s' "$doc"
}

# case_ <label> <PASS|FAIL> <override>...
case_() {
	local label="$1" want="$2" rc got
	shift 2
	NEEDS="$(needs_json "$@")" scripts/ci-required-verdict.sh >/dev/null 2>&1
	rc=$?
	got=PASS
	[ "$rc" -eq 0 ] || got=FAIL
	checks=$((checks + 1))
	[ "$got" = "$want" ] || {
		echo "FAIL $label: got $got, want $want"
		fail=1
	}
}

go_ready="changes.go=true plan.has_tests=true"
all_heavy="changes.go=true plan.has_tests=true changes.kit=true changes.tfprovider=true changes.web=full changes.vitest=changed changes.installer=true"
heavy_skipped="test-go=skipped kit=skipped tfprovider=skipped web-test=skipped install-sh=skipped install-cli-macos=skipped install-cli-windows=skipped test-api=skipped test-rest=skipped"

case_ "ready PR, nothing affected, all green" PASS
case_ "ready PR, every needed lane success" PASS $all_heavy
case_ "draft, heavy lanes skipped" PASS changes.draft=true $all_heavy $heavy_skipped
case_ "draft, cheap lane build-vet failed" FAIL changes.draft=true $all_heavy $heavy_skipped build-vet=failure
case_ "draft, lint failed" FAIL changes.draft=true lint=failure
case_ "draft, web-check failed" FAIL changes.draft=true web-check=failure
case_ "draft, cheap lane cancelled" FAIL changes.draft=true lint=cancelled
case_ "draft, a heavy lane failed" FAIL changes.draft=true test-go=failure
case_ "draft, changes failed" FAIL changes.draft=true changes=failure
case_ "ready, tests needed but test-go skipped" FAIL $go_ready test-go=skipped
case_ "ready, go changed, planner says no tests, test-go skipped" PASS changes.go=true plan.has_tests=false test-go=skipped
case_ "ready, docs only, lanes skipped as not affected" PASS test-go=skipped kit=skipped tfprovider=skipped web-test=skipped install-sh=skipped test-api=skipped test-rest=skipped
case_ "ready, web full changed, web-test skipped" FAIL changes.web=full changes.vitest=changed web-test=skipped
case_ "ready, web full vitest full, web-test skipped" FAIL changes.web=full changes.vitest=full web-test=skipped
case_ "ready, web full vitest none, web-test skipped" PASS changes.web=full changes.vitest=none web-test=skipped
case_ "ready, web docs only, web-test skipped" PASS changes.web=docs web-test=skipped
case_ "ready, web full, web-test success" PASS changes.web=full changes.vitest=changed
case_ "ready, kit changed, kit skipped" FAIL changes.kit=true kit=skipped
case_ "ready, kit changed, kit success" PASS changes.kit=true
case_ "ready, tfprovider changed, tfprovider skipped" FAIL changes.tfprovider=true tfprovider=skipped
case_ "ready, installer changed, install-sh skipped" FAIL changes.installer=true install-sh=skipped
case_ "ready, installer changed, macos skipped" FAIL changes.installer=true install-cli-macos=skipped
case_ "ready, installer changed, windows skipped" FAIL changes.installer=true install-cli-windows=skipped
case_ "ready, installer changed, all success" PASS changes.installer=true
case_ "ready, installer not changed, installer lanes skipped" PASS install-sh=skipped install-cli-macos=skipped install-cli-windows=skipped
case_ "cancelled lane" FAIL test-go=cancelled
case_ "cancelled cheap lane" FAIL lint=cancelled
case_ "failed test-go" FAIL test-go=failure
case_ "failed web-test" FAIL web-test=failure
case_ "failed kit" FAIL kit=failure
case_ "failed install-sh" FAIL install-sh=failure
case_ "failed test-api" FAIL test-api=failure
case_ "failed test-rest" FAIL test-rest=failure
case_ "failed plan" FAIL plan=failure
case_ "push to main, coverage-gate absent from needs (informational)" PASS changes.go=true plan.has_tests=true
case_ "push to main, test-go failed" FAIL changes.go=true plan.has_tests=true test-go=failure
case_ "push, empty draft output, everything green" PASS changes.draft= changes.go=true plan.has_tests=true
case_ "merge_group, all green" PASS $all_heavy
case_ "merge_group, test-go skipped though needed" FAIL $go_ready test-go=skipped
case_ "merge_group, lint failed" FAIL lint=failure
case_ "release-pacing skipped on a normal PR" PASS release-pacing=skipped
case_ "release-pacing success on a release PR" PASS release-pacing=success
case_ "release-pacing failed on a release PR" FAIL release-pacing=failure
case_ "release-pacing cancelled" FAIL release-pacing=cancelled
case_ "changes skipped" FAIL changes=skipped
case_ "changes cancelled" FAIL changes=cancelled
case_ "changes job missing from needs" FAIL changes=DROP
case_ "changes outputs empty, draft unset, all else green" PASS changes.go= changes.draft=
case_ "changes failed with empty outputs" FAIL changes=failure changes.go= changes.draft= changes.web= changes.kit=
case_ "changes skipped with empty outputs and tests skipped" FAIL changes=skipped changes.go= changes.draft= test-go=skipped
case_ "CI_ON_DRAFTS=true: draft output false, test-go skipped though needed" FAIL changes.draft=false $go_ready test-go=skipped
case_ "CI_ON_DRAFTS=true: draft output false, all lanes run" PASS changes.draft=false $all_heavy
case_ "CI_ON_DRAFTS unset: draft output true, test-go skipped" PASS changes.draft=true $go_ready test-go=skipped
case_ "unexpected result value on a lane" FAIL test-go=timed_out
case_ "empty result value on a lane" FAIL lint=
case_ "unexpected result on release-pacing" FAIL release-pacing=neutral
case_ "unexpected result on changes" FAIL changes=weird
case_ "needed lane missing from needs, tests needed" FAIL $go_ready test-go=DROP
case_ "draft output garbage is treated as ready" FAIL changes.draft=yes $go_ready test-go=skipped
case_ "draft output capitalised True is not draft" FAIL changes.draft=True $go_ready test-go=skipped
case_ "go output garbage does not demand test-go" PASS changes.go=yes plan.has_tests=true test-go=skipped
case_ "has_tests garbage does not demand test-go" PASS changes.go=true plan.has_tests=maybe test-go=skipped
case_ "vitest garbage does not demand web-test" PASS changes.web=full changes.vitest=maybe web-test=skipped

# Non-object needs must fail closed rather than crash into a pass.
for bad in '' 'null' '[]' '"x"' 'not json'; do
	checks=$((checks + 1))
	if [ -z "$bad" ]; then
		env -u NEEDS scripts/ci-required-verdict.sh >/dev/null 2>&1 && {
			echo "FAIL unset NEEDS passed"
			fail=1
		}
	else
		NEEDS="$bad" scripts/ci-required-verdict.sh >/dev/null 2>&1 && {
			echo "FAIL needs '$bad' passed"
			fail=1
		}
	fi
done

# The verdict word and a reason line must reach stdout and the job summary.
summary="$(mktemp)"
trap 'rm -f "$summary"' EXIT
out="$(NEEDS="$(needs_json $go_ready test-go=skipped)" GITHUB_STEP_SUMMARY="$summary" scripts/ci-required-verdict.sh 2>&1)"
checks=$((checks + 3))
grep -qx FAIL <<<"$out" || { echo "FAIL verdict word missing"; fail=1; }
grep -q "test-go was skipped" <<<"$out" || { echo "FAIL reason line missing"; fail=1; }
grep -q "test-go was skipped" "$summary" || { echo "FAIL summary reason missing"; fail=1; }

if [ "$fail" -ne 0 ]; then
	echo "test-ci-required-verdict: FAILED"
	exit 1
fi
echo "test-ci-required-verdict: $checks checks passed"
