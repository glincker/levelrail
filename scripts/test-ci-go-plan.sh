#!/usr/bin/env bash
# Table tests for scripts/ci-go-plan.sh: lane layout, the live-suite gate,
# and the invariant that every planned lane is named by a required check.
#
# Usage: scripts/test-ci-go-plan.sh
set -uo pipefail

cd "$(git rev-parse --show-toplevel)" || exit 2

fail=0
checks=0

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT
n_cases=0

# run <label> <env-assignment|-> <expect>... -- <path>...
# Each expect is key=value (exact output line) or has:lane / lacks:lane
# (lane list) or check:Test (name) / nocheck:Test (name) (required checks).
# Cases run concurrently (PLAN_TEST_JOBS, default 8); PLAN_TEST_FILTER is a
# regex on the label to run a subset locally.
run() {
	if [ -n "${PLAN_TEST_FILTER:-}" ] && ! [[ "$1" =~ $PLAN_TEST_FILTER ]]; then
		return
	fi
	n_cases=$((n_cases + 1))
	local idx=$n_cases
	(
		# shellcheck disable=SC2030 # per-case subshell; the count is handed back through a file
		checks=0
		run_case "$@"
		echo "$checks" >"$tmp/$idx.checks"
	) >"$tmp/$idx.out" 2>&1 &
	while [ "$(jobs -rp | wc -l)" -ge "${PLAN_TEST_JOBS:-8}" ]; do sleep 0.2; done
}

collect() {
	local f
	wait
	for f in "$tmp"/*.out; do
		[ -s "$f" ] || continue
		cat "$f"
		fail=1
	done
	for f in "$tmp"/*.checks; do
		# shellcheck disable=SC2031
		[ -f "$f" ] && checks=$((checks + $(cat "$f")))
	done
}

run_case() {
	local label="$1" env_kv="$2"
	shift 2
	local expects=()
	while [ "$#" -gt 0 ] && [ "$1" != "--" ]; do
		expects+=("$1")
		shift
	done
	shift
	local out
	if [ "$env_kv" = "-" ]; then
		out="$(printf '%s\n' "$@" | scripts/ci-go-plan.sh --files 2>/dev/null)"
	else
		# shellcheck disable=SC2086 # env_kv is one or more KEY=value words
		out="$(printf '%s\n' "$@" | env $env_kv scripts/ci-go-plan.sh --files 2>/dev/null)"
	fi
	local lanes line_checks e
	lanes=" $(sed -n 's/^lanes=//p' <<<"$out") "
	line_checks="$(sed -n -e 's/^api_checks=//p' -e 's/^rest_checks=//p' <<<"$out")"
	for e in "${expects[@]}"; do
		checks=$((checks + 1))
		case "$e" in
		has:*) [[ "$lanes" == *" ${e#has:} "* ]] || { echo "FAIL $label: lane ${e#has:} missing (lanes:$lanes)"; fail=1; } ;;
		lacks:*) [[ "$lanes" != *" ${e#lacks:} "* ]] || { echo "FAIL $label: lane ${e#lacks:} present (lanes:$lanes)"; fail=1; } ;;
		check:*) grep -qF "\"${e#check:}\"" <<<"$line_checks" || { echo "FAIL $label: check ${e#check:} missing"; fail=1; } ;;
		nocheck:*) ! grep -qF "\"${e#nocheck:}\"" <<<"$line_checks" || { echo "FAIL $label: check ${e#nocheck:} present"; fail=1; } ;;
		*) grep -qxF "$e" <<<"$out" || { echo "FAIL $label: want line $e"; fail=1; } ;;
		esac
	done
	# Every lane must be gated by a required check, or it could fail unseen.
	local lane
	# shellcheck disable=SC2013 # lane names never contain spaces
	for lane in $(sed -n 's/^lanes=//p' <<<"$out"); do
		checks=$((checks + 1))
		grep -qF "\"Test ($lane)\"" <<<"$line_checks" || { echo "FAIL $label: lane $lane has no required check"; fail=1; }
	done
}

run "docs only" - "scope=none" "has_tests=false" "live=smoke" -- docs/ci.md README.md
run "docs only with label" CI_LIVE_LABEL=true "scope=none" "has_tests=false" -- docs/ci.md
run "api change runs e2e but not the fleet or reconcile lanes" - "scope=some" "live=smoke" has:api-1 has:e2e lacks:e2e-fleet lacks:e2e-reconcile \
	check:"Test (e2e)" nocheck:"Test (e2e-fleet)" nocheck:"Test (e2e-reconcile)" -- internal/api/apps.go
run "api change with ci:live label is full" CI_LIVE_LABEL=true "live=full" "live_reason=PR label ci:live" has:e2e-fleet has:e2e \
	check:"Test (e2e-fleet)" check:"Test (e2e)" -- internal/api/apps.go
run "ingress change runs e2e only" - "live=full" has:e2e lacks:e2e-fleet lacks:e2e-reconcile \
	check:"Test (e2e)" nocheck:"Test (e2e-fleet)" nocheck:"Test (e2e-reconcile)" -- internal/ingress/caddy.go
run "reconcile/ingress change runs e2e only" - "live=full" has:e2e lacks:e2e-fleet lacks:e2e-reconcile \
	-- internal/reconcile/ingress/controller.go
run "reconcile/ingress plus reconcile runs fleet" - "live=full" has:e2e-fleet has:e2e-reconcile \
	-- internal/reconcile/ingress/controller.go internal/reconcile/application/controller.go
run "agent change runs reconcile e2e, not single-node e2e" - "live=full" has:e2e-reconcile lacks:e2e -- internal/agent/agent.go
run "network change runs reconcile e2e, not single-node e2e" - "live=full" has:e2e-reconcile lacks:e2e -- internal/network/mesh.go
run "docker change runs e2e only" - "live=full" has:e2e lacks:e2e-fleet lacks:e2e-reconcile -- internal/docker/client.go
run "deploy change runs e2e, not full live" - "live=smoke" has:e2e lacks:e2e-fleet lacks:e2e-reconcile -- internal/deploy/deploy.go
run "pipeline script change plus api runs fleet and e2e" - "live=full" has:e2e-fleet has:e2e -- scripts/ci-go-plan.sh internal/api/apps.go
run "reconcile change is full" - "live=full" has:e2e-fleet has:e2e has:e2e-reconcile \
	check:"Test (e2e-fleet)" check:"Test (e2e-reconcile)" -- internal/reconcile/application/controller.go
run "catalog data change is live=full but runs no e2e lane" - "live=full" lacks:e2e-fleet lacks:e2e -- internal/catalog/templates_ai.go
run "e2e test change is full" - "live=full" has:e2e-fleet has:e2e -- test/e2e/template_fleet_test.go
run "e2e testenv change is full" - "live=full" has:e2e-reconcile -- test/e2e/testenv/testenv.go
run "go.mod is a full run" - "scope=all" "live=full" "live_reason=full run" has:docker has:e2e-fleet -- go.mod
run "additive migration scopes to the store and its table users" - "scope=some" lacks:api-1 lacks:e2e -- internal/store/migrations/0404_database_network_rule_notes.sql
run "data-rewriting migration is a full run" - "scope=all" has:api-1 -- internal/store/migrations/0375_global_environments.sql
run "pipeline-only full run keeps live suites in smoke mode" CI_LIVE_SMOKE=true "scope=all" "live=smoke" lacks:e2e-fleet -- go.mod
run "pipeline-only full run with ci:live label is full" "CI_LIVE_SMOKE=true CI_LIVE_LABEL=true" "scope=all" "live=full" has:e2e-fleet -- go.mod
run "web only has no go lanes" - "scope=none" "has_tests=false" -- web/src/App.tsx

collect

# Whole-tree scanners are selected by any non-test Go change, never by docs.
expect_pkg() { # label want(yes|no) pkg path...
	local label="$1" want="$2" pkg="$3" out
	shift 3
	checks=$((checks + 1))
	out="$(printf '%s\n' "$@" | scripts/affected-go-packages.sh --stdin 2>/dev/null)"
	if grep -qxF "$pkg" <<<"$out"; then
		[ "$want" = yes ] || { echo "FAIL $label: $pkg selected"; fail=1; }
	else
		[ "$want" = no ] || { echo "FAIL $label: $pkg missing"; fail=1; }
	fi
}
guard=github.com/GLINCKER/levelrail/test/execgate
expect_pkg "go change selects execgate" yes "$guard" internal/spec/discover.go
expect_pkg "test-only change skips execgate" no "$guard" internal/spec/discover_test.go
expect_pkg "docs change skips execgate" no "$guard" docs/ci.md

if [ "$fail" -ne 0 ]; then
	echo "test-ci-go-plan: FAILED"
	exit 1
fi
echo "test-ci-go-plan: $checks checks passed"
