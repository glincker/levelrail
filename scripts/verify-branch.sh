#!/usr/bin/env bash
# Verifies only what this branch changed since its merge base with the release line.
# Go steps are scoped to affected packages, web steps to changed files; the full suite never runs.
# Usage: scripts/verify-branch.sh [base-ref]     (default origin/release/v0.8)
# Environment: LEVELRAIL_VERIFY_JOBS caps parallel steps (default 2).
set -o pipefail

root="$(git rev-parse --show-toplevel)" || exit 2
cd "$root" || exit 2

base_ref="${1:-origin/release/v0.8}"
jobs="${LEVELRAIL_VERIFY_JOBS:-2}"
module="github.com/GLINCKER/levelrail"
log_dir="$(git rev-parse --git-dir)/verify"
rm -rf "$log_dir" && mkdir -p "$log_dir" || exit 2
results="$log_dir/results.txt"
: >"$results"

merge_base="$(git merge-base HEAD "$base_ref" 2>/dev/null)" || {
	echo "FAIL base: no merge base with $base_ref (run: git fetch origin)"
	exit 2
}
changed="$({ git diff --name-only --diff-filter=ACMRD "$merge_base"; git ls-files --others --exclude-standard; } | sort -u)"
if [ -z "$changed" ]; then
	echo "PASS nothing changed vs $base_ref"
	exit 0
fi

pids=()
record() { printf '%s\t%s\t%s\n' "$1" "$2" "$3" >>"$results"; }

# Launches a step in a bounded slot; $1 name, $2 seconds, $3 dir, $4.. command.
run_step() {
	local name="$1" secs="$2" dir="$3" slug
	shift 3
	slug="$(printf '%s' "$name" | tr -c 'A-Za-z0-9' '_')"
	while [ "${#pids[@]}" -ge "$jobs" ]; do
		wait "${pids[0]}"
		pids=("${pids[@]:1}")
	done
	(
		cd "$dir" && timeout -k 10 "$secs" "$@" >"$log_dir/$slug.log" 2>&1
		echo $? >"$log_dir/$slug.status"
	) &
	pids+=("$!")
	record "$name" RUN "$slug"
}

# ---- Go scope ----
go_files="$(printf '%s\n' "$changed" | grep -E '\.go$|^go\.(mod|sum)$')"
affected=""
if [ -n "$go_files" ]; then
	affected="$(printf '%s\n' "$changed" | scripts/affected-go-packages.sh --stdin)"
	if [ "$affected" = "ALL" ]; then
		# The import graph cannot scope go.mod or similar: fall back to directly changed packages.
		affected="$(printf '%s\n' "$go_files" | xargs -n1 dirname | sort -u |
			while IFS= read -r d; do if [ "$d" = . ]; then echo "$module"; else echo "$module/$d"; fi; done)"
	fi
fi

if [ -z "$affected" ]; then
	record go-build SKIP "no Go package affected"
	record go-vet SKIP "no Go package affected"
	record go-lint SKIP "no Go package affected"
	record go-test SKIP "no Go package affected"
	record go-test-api SKIP "no Go package affected"
else
	api_pkg="$module/internal/api"
	other_pkgs="$(printf '%s\n' "$affected" | grep -vxF "$api_pkg")"
	lint_dirs="$(printf '%s\n' "$affected" | sed "s#^$module#.#")"

	# Test-only packages (no non-test files) cannot be built; go vet still covers them.
	buildable="$(go list -e -f '{{if or .GoFiles .CgoFiles}}{{.ImportPath}}{{end}}' $affected 2>/dev/null | grep . || true)"
	if [ -n "$buildable" ]; then
		# shellcheck disable=SC2086 # package lists are intentionally word-split
		run_step go-build 600 . go build $buildable
	else
		record go-build SKIP "only test-only packages affected"
	fi
	# shellcheck disable=SC2086
	run_step go-vet 600 . go vet $affected
	# shellcheck disable=SC2086
	run_step go-lint 900 . golangci-lint run $lint_dirs
	if [ -n "$other_pkgs" ]; then
		# shellcheck disable=SC2086
		run_step go-test 900 . go test -short -timeout 10m $other_pkgs
	else
		record go-test SKIP "only internal/api affected"
	fi

	if printf '%s\n' "$affected" | grep -qxF "$api_pkg"; then
		api_re="$(printf '%s\n' "$changed" | grep '^internal/api/.*_test\.go$' |
			while IFS= read -r f; do grep -ho '^func \(Test\|Fuzz\|Example\)[A-Za-z0-9_]*' "$f" 2>/dev/null | sed 's/^func //'; done |
			sort -u | paste -sd'|' -)"
		if [ -n "$other_pkgs" ]; then
			# shellcheck disable=SC2086
			narrowed="$(scripts/affected-api-tests.sh $other_pkgs 2>/dev/null || echo ALL)"
		else
			narrowed="ALL"
		fi
		if [ "$narrowed" = ALL ] || [ -z "$narrowed" ]; then
			record go-test-api SKIP "NOT VERIFIED: internal/api could not be narrowed, run scripts/test-fast.sh -r '<regex>' by hand"
		else
			api_re="${api_re:+$api_re|}$narrowed"
			run_step go-test-api 900 . scripts/test-fast.sh -r "^($api_re)\$"
		fi
	else
		record go-test-api SKIP "internal/api not in affected set"
	fi
fi

# ---- Web scope ----
web_changed="$(printf '%s\n' "$changed" | grep '^web/' || true)"
if [ -z "$web_changed" ]; then
	record web-vitest SKIP "no web files changed"
	record web-tsc SKIP "no web files changed"
elif [ ! -d web/node_modules ]; then
	record web-vitest FAIL "web/node_modules missing (run: cd web && npm ci)"
	record web-tsc FAIL "web/node_modules missing (run: cd web && npm ci)"
else
	web_src="$(printf '%s\n' "$web_changed" | grep -E '^web/src/.*\.(ts|tsx)$' | sed 's#^web/##')"
	if [ -n "$web_src" ]; then
		# shellcheck disable=SC2086
		run_step web-vitest 900 web npx vitest related --run $web_src --reporter=dot
	else
		record web-vitest SKIP "no web/src TypeScript files changed"
	fi
	run_step web-tsc 900 web npx tsc -b
fi

for p in "${pids[@]}"; do wait "$p"; done

# ---- Report ----
fail_count=0 pass_count=0 skip_count=0
while IFS=$'\t' read -r name verdict detail; do
	case "$verdict" in
	SKIP)
		skip_count=$((skip_count + 1))
		echo "SKIP $name ($detail)"
		;;
	FAIL)
		fail_count=$((fail_count + 1))
		echo "FAIL $name ($detail)"
		;;
	RUN)
		slug="$detail"
		status="$(cat "$log_dir/$slug.status" 2>/dev/null || echo 1)"
		if [ "$status" = 0 ]; then
			pass_count=$((pass_count + 1))
			echo "PASS $name"
		else
			fail_count=$((fail_count + 1))
			if [ "$status" = 124 ]; then reason="timed out"; else reason="exit $status"; fi
			echo "FAIL $name ($reason, log: $log_dir/$slug.log)"
			tail -n 10 "$log_dir/$slug.log" | sed 's/^/    /'
		fi
		;;
	esac
done <"$results"

echo "verify-branch vs $base_ref: $pass_count passed, $fail_count failed, $skip_count skipped"
[ "$fail_count" -eq 0 ]
