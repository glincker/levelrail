#!/usr/bin/env bash
# Runs a CI Go test lane through gotestsum: bounded reruns of failed tests,
# JUnit/JSON output, merged coverage across reruns, quarantine handling, and
# a job summary naming every test that passed only on retry.
#
# Usage: scripts/ci-go-test.sh <lane-name> <package>...
#
# Environment:
#   GO_TEST_FLAGS       extra go test flags (e.g. "-short -timeout=13m")
#   COVERPROFILE        write a merged coverage profile here (optional)
#   RERUNS              reruns per failed test (default 2, 0 disables)
#   RERUN_MAX_FAILURES  skip reruns entirely above this many failures (default 5)
#   SHARD               "i/n": run only every n-th top-level test (single package only)
#   QUARANTINE          "skip" (default) skips .github/flaky-tests.txt entries,
#                       "only" runs just those entries
#   RESULTS_DIR         output directory (default test-results)
set -uo pipefail

lane="${1:?usage: ci-go-test.sh <lane-name> <package>...}"
shift
[ "$#" -gt 0 ] || {
	echo "ci-go-test.sh: no packages given" >&2
	exit 2
}
packages=("$@")

root="$(git rev-parse --show-toplevel)"
cd "$root" || exit 2

reruns="${RERUNS:-2}"
max_failures="${RERUN_MAX_FAILURES:-5}"
quarantine_mode="${QUARANTINE:-skip}"
out="${RESULTS_DIR:-test-results}/$lane"
mkdir -p "$out"
: >"$out/flaky.txt"
: >"$out/failed.txt"

read -r -a flags <<<"${GO_TEST_FLAGS:-}"
summary="${GITHUB_STEP_SUMMARY:-/dev/null}"

pkg_paths="$(go list "${packages[@]}")" || exit 2

quarantined=()
quarantined_pkgs=()
if [ -f .github/flaky-tests.txt ]; then
	while read -r qpkg qtest _; do
		case "$qpkg" in '' | '#'*) continue ;; esac
		if grep -qxF "$qpkg" <<<"$pkg_paths"; then
			quarantined+=("$qtest")
			quarantined_pkgs+=("$qpkg")
		fi
	done <.github/flaky-tests.txt
fi
quarantine_re=""
if [ "${#quarantined[@]}" -gt 0 ]; then
	quarantine_re="^($(
		IFS='|'
		echo "${quarantined[*]}"
	))\$"
fi

case "$quarantine_mode" in
skip)
	[ -n "$quarantine_re" ] && flags+=("-skip=$quarantine_re")
	;;
only)
	if [ -z "$quarantine_re" ]; then
		echo "No quarantined tests in lane $lane." | tee -a "$summary"
		exit 0
	fi
	flags+=("-run=$quarantine_re")
	mapfile -t packages < <(printf '%s\n' "${quarantined_pkgs[@]}" | sort -u)
	;;
*)
	echo "ci-go-test.sh: QUARANTINE must be skip or only" >&2
	exit 2
	;;
esac

if [ -n "${SHARD:-}" ]; then
	if [ "${#packages[@]}" -ne 1 ]; then
		echo "ci-go-test.sh: SHARD needs exactly one package" >&2
		exit 2
	fi
	shard_i="${SHARD%/*}"
	shard_n="${SHARD#*/}"
	names="$(go test -list '.*' "${packages[0]}" | grep -E '^(Test|Example|Fuzz)' | sort)" || exit 2
	shard_names="$(awk -v i="$shard_i" -v n="$shard_n" '(NR - 1) % n == i - 1' <<<"$names" | paste -sd'|' -)"
	if [ -z "$shard_names" ]; then
		echo "Shard $SHARD of $lane has no tests." | tee -a "$summary"
		exit 0
	fi
	flags+=("-run=^($shard_names)\$")
fi

covdir=""
trailing=()
if [ -n "${COVERPROFILE:-}" ]; then
	# Raw coverage data accumulates across reruns; -coverprofile would be
	# overwritten by each rerun's partial profile.
	covdir="$(mktemp -d)"
	flags+=("-cover" "-covermode=set")
	trailing=("-args" "-test.gocoverdir=$covdir")
fi

gotestsum_args=(
	--format pkgname-and-test-fails
	--jsonfile "$out/events.json"
	--junitfile "$out/junit.xml"
	--packages "${packages[*]}"
)
if [ "$reruns" -gt 0 ]; then
	gotestsum_args+=(
		--rerun-fails="$reruns"
		--rerun-fails-max-failures="$max_failures"
		--rerun-fails-report "$out/reruns.txt"
		--rerun-fails-abort-on-data-race
	)
fi

gotestsum "${gotestsum_args[@]}" -- "${flags[@]}" "${trailing[@]}"
status=$?

if [ -n "$covdir" ]; then
	if go tool covdata textfmt -i="$covdir" -o "$COVERPROFILE"; then
		# Packages without test files never run a binary, so they leave no
		# raw data; plain -coverprofile still reports them at 0%.
		notest="$(go list -f '{{if and (not .TestGoFiles) (not .XTestGoFiles)}}{{.ImportPath}}{{end}}' "${packages[@]}")"
		if [ -n "$notest" ]; then
			tmp="$(mktemp)"
			# shellcheck disable=SC2086 # one import path per word
			go test -coverprofile="$tmp" $notest >/dev/null && tail -n +2 "$tmp" >>"$COVERPROFILE"
			rm -f "$tmp"
		fi
	else
		echo "ci-go-test.sh: no coverage data recorded" >&2
		[ "$status" -eq 0 ] && status=1
	fi
	rm -rf "$covdir"
fi

# A test that both failed and passed is flaky; one that only failed is broken.
# Parents of a flaky subtest are dropped so each flake is listed once.
jq -rs '
	[.[] | select(.Test != null and (.Action == "pass" or .Action == "fail"))]
	| group_by([.Package, .Test])
	| map({pkg: .[0].Package, test: .[0].Test,
	       pass: map(select(.Action == "pass")) | length,
	       fail: map(select(.Action == "fail")) | length})
	| map(select(.fail > 0)) as $f
	| $f[]
	| . as $t
	| select([$f[] | select(.pkg == $t.pkg and (.test | startswith($t.test + "/")))] | length == 0)
	| "\(if .pass > 0 then "flaky" else "failed" end) \(.pkg) \(.test) \(.fail) \(.pass + .fail)"
' "$out/events.json" 2>/dev/null | while read -r kind pkg test fails runs; do
	echo "$pkg $test $fails/$runs" >>"$out/$kind.txt"
done

{
	echo "### Go tests: $lane"
	if [ -s "$out/flaky.txt" ]; then
		echo
		echo "Passed only on retry (flaky, see CONTRIBUTING.md#flaky-tests):"
		echo
		echo "| Package | Test | Failed / total attempts |"
		echo "| --- | --- | --- |"
		awk '{printf "| `%s` | `%s` | %s |\n", $1, $2, $3}' "$out/flaky.txt"
	fi
	if [ -s "$out/failed.txt" ]; then
		echo
		echo "Failed on every attempt:"
		echo
		awk '{printf "- `%s` `%s`\n", $1, $2}' "$out/failed.txt"
	fi
	if [ ! -s "$out/flaky.txt" ] && [ ! -s "$out/failed.txt" ]; then
		echo
		echo "Every test passed on its first attempt."
	fi
} >>"$summary"

while read -r pkg test attempts; do
	echo "::warning title=Flaky test::$pkg $test failed $attempts attempts and passed the rest"
done <"$out/flaky.txt"

exit "$status"
