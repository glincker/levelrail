#!/bin/sh
# Table tests for scripts/ci-changes.sh (which CI areas a change class runs),
# scripts/ci-shard-names.sh (shard partition) and scripts/check-lane-results.sh
# (a skipped or missing lane fails the check that gates `CI required`).
#
# Usage: scripts/test-ci-changes.sh
set -u

cd "$(git rev-parse --show-toplevel)" || exit 2

fail=0
checks=0
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

# case <label> "<key=value>..." -- <path>...
case_() {
	label="$1"
	want="$2"
	shift 3
	out="$(printf '%s\n' "$@" | scripts/ci-changes.sh --files 2>/dev/null)"
	for kv in $want; do
		checks=$((checks + 1))
		printf '%s\n' "$out" | grep -qx "$kv" || {
			echo "FAIL $label: want $kv"
			fail=1
		}
	done
}

none="go=false web=none lint=none kit=false tfprovider=false installer=false workflows=false full=false"

case_ "docs only builds the help manifest, no Go" "go=false web=docs lint=none full=false kit=false installer=false" -- docs/ci.md
case_ "docs excluded from the manifest need no web job" "web=none full=false" -- docs/changelog/v1.md
case_ "markdown outside docs runs nothing" "$none" -- README.md adr/001-x.md
case_ "web source only: web job, no Go" "go=false web=full vitest=changed lint=none kit=false" -- web/src/App.tsx
case_ "web lockfile runs the full web suite, no Go" "go=false web=full vitest=full" -- web/package-lock.json
case_ "unreferenced script runs nothing" "$none" -- scripts/smoke.sh
case_ "go only: no web, no kit" "go=true web=none kit=false tfprovider=false lint=changed" -- internal/brand/brand.go
case_ "other workflow: actionlint only" "workflows=true go=false web=none full=false" -- .github/workflows/nightly.yml
case_ "dependabot config runs nothing" "$none" -- .github/dependabot.yml
case_ "go.mod runs every Go lane and lint" "go=true go_full=true lint=all" -- go.mod
case_ "go.sum runs every Go lane and lint" "go=true go_full=true lint=all" -- go.sum
case_ "ci.yml runs everything" "full=true go_full=true web=full kit=true tfprovider=true installer=true workflows=true lint=all planner=true" -- .github/workflows/ci.yml
case_ "the change detector runs everything" "full=true go_full=true web=full" -- scripts/ci-changes.sh
case_ "the required verdict script runs everything and the planner tests" "full=true go_full=true planner=true" -- scripts/ci-required-verdict.sh
case_ "the verdict table test runs everything and the planner tests" "full=true planner=true" -- scripts/test-ci-required-verdict.sh
case_ "the Go planner reruns every Go lane, not web" "go=true go_full=true web=none full=false" -- scripts/ci-go-plan.sh
case_ "the shard splitter reruns every Go lane" "go=true go_full=true web=none" -- scripts/ci-shard-names.sh
case_ "planner tests alone run the planner tests, no Go lane" "go=true go_full=false planner=true web=none" -- scripts/test-ci-go-plan.sh
case_ "shard weights alone run the planner tests" "go=true go_full=false planner=true" -- scripts/ci-test-weights.txt
case_ "the lane runner (rerun policy) reruns every Go lane" "go=true go_full=true web=none full=false" -- scripts/ci-go-test.sh
case_ "nightly workflow edits run actionlint only, no Go lane" "workflows=true go=false go_full=false full=false" -- .github/workflows/nightly.yml
case_ "a moved live test still runs Go for its package" "go=true go_full=false full=false" -- test/e2e/loadbalancer_test.go
case_ "Go setup action reruns every Go lane" "go=true go_full=true" -- .github/actions/setup-go-cached/action.yml
case_ "docs read by a Go test still run Go" "go=true web=docs" -- docs/api-reference.md
case_ "docs json embedded or read by Go runs Go" "go=true" -- docs/public/openapi.json
case_ "embedded data file runs Go" "go=true web=none" -- internal/dbupgrade/catalog.json
case_ "web embed.go runs Go, web source does not" "go=true web=none" -- web/embed.go
case_ "kit module change runs the kit job" "go=true kit=true" -- kit/upgrade/preflight.go
case_ "terraform provider change runs its own job only" "tfprovider=true go=false web=none" -- integrations/terraform-provider/main.go
case_ "installer change runs the installer job" "installer=true" -- install.sh
case_ "packaging change runs the installer job" "installer=true" -- deploy/packaging/nfpm/agent.yaml
case_ "dockerfile move target runs nothing, as the root did" "$none" -- deploy/docker/Dockerfile.levelrail
case_ "release-please config runs nothing, as the root did" "$none" -- .github/release-please/release-please-config.json .github/release-please/.release-please-manifest.json
case_ "contributing and security files run nothing" "$none" -- .github/CONTRIBUTING.md .github/SECURITY.md
case_ "dev fixtures file runs Go (read by api tests)" "go=true" -- test/fixtures/dev-fixtures.yml

# Shard partition: every name lands in exactly one shard, deterministically.
printf '%s\n' TestA TestB TestC TestD TestE TestF TestG >"$tmp/names"
: >"$tmp/all"
for i in 1 2 3; do
	scripts/ci-shard-names.sh "$i" 3 <"$tmp/names" >"$tmp/s$i"
	cat "$tmp/s$i" >>"$tmp/all"
done
checks=$((checks + 1))
sort "$tmp/all" | cmp -s - "$tmp/names" || {
	echo "FAIL shard partition: shards do not cover each name exactly once"
	fail=1
}
checks=$((checks + 1))
scripts/ci-shard-names.sh 2 3 <"$tmp/names" | cmp -s - "$tmp/s2" || {
	echo "FAIL shard partition: not deterministic"
	fail=1
}
printf '# w\nTestA 50\nTestB 40\nTestC 1\n' >"$tmp/w"
checks=$((checks + 1))
[ "$(scripts/ci-shard-names.sh 1 2 "$tmp/w" <"$tmp/names" | head -n1)" = TestA ] || {
	echo "FAIL shard weights: heaviest test not placed first"
	fail=1
}
checks=$((checks + 1))
[ "$(scripts/ci-shard-names.sh 2 2 "$tmp/w" <"$tmp/names" | head -n1)" = TestB ] || {
	echo "FAIL shard weights: second heaviest test not on the other shard"
	fail=1
}

# Lane aggregation: a lane that is skipped, missing or failed must fail the
# gate that `CI required` depends on. A fake gh serves the job list.
mkdir -p "$tmp/bin"
cat >"$tmp/bin/gh" <<'GH'
#!/bin/sh
cat "$FAKE_JOBS"
GH
chmod +x "$tmp/bin/gh"
lanes() { # expect <label> <0|1> <job list>
	label="$1"
	want="$2"
	printf '%b' "$3" >"$tmp/jobs"
	rc=0
	PATH="$tmp/bin:$PATH" FAKE_JOBS="$tmp/jobs" GITHUB_REPOSITORY=x/y GITHUB_RUN_ID=1 \
		scripts/check-lane-results.sh "Test (e2e-reconcile)" "Test (e2e-reconcile-2)" >/dev/null 2>&1 || rc=1
	checks=$((checks + 1))
	[ "$rc" = "$want" ] || {
		echo "FAIL lane aggregation: $label (exit $rc, want $want)"
		fail=1
	}
}
lanes "both lanes passed" 0 'Test (e2e-reconcile)\tsuccess\nTest (e2e-reconcile-2)\tsuccess\n'
lanes "second lane wrongly skipped" 1 'Test (e2e-reconcile)\tsuccess\nTest (e2e-reconcile-2)\tskipped\n'
lanes "second lane missing" 1 'Test (e2e-reconcile)\tsuccess\n'
lanes "second lane failed" 1 'Test (e2e-reconcile)\tsuccess\nTest (e2e-reconcile-2)\tfailure\n'

if [ "$fail" -ne 0 ]; then
	echo "test-ci-changes: FAILED"
	exit 1
fi
echo "test-ci-changes: $checks checks passed"
