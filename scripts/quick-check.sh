#!/usr/bin/env bash
# The minimal checks for what you changed, in parallel, failing lines only.
#
# Usage: scripts/quick-check.sh [base-ref]
#   base-ref  default: the nearest origin/release/* or origin/main (LEVELRAIL_QUICK_BASE)
#
# Covers committed, staged, unstaged and untracked changes against the merge base:
#   Go    go vet + go test -short on changed packages plus their reverse deps (only
#         while that set stays small), golangci-lint on changed packages only.
#         internal/api runs just the tests the change can reach (scripts/test-fast.sh).
#   Web   routeTree refresh, tsc -b, eslint and vitest related on changed files.
#   API   OpenAPI/api-reference drift, only when internal/api sources changed.
# Env: QUICK_MAX_PKGS (reverse-dep cap, default 12), QUICK_JOBS (parallel steps, default 3).
# Not a replacement for CI: it never runs the coverage gate, e2e or docker lanes.
set -o pipefail

root="$(git rev-parse --show-toplevel)" || exit 2
cd "$root" || exit 2
module="github.com/GLINCKER/levelrail"
api_pkg="$module/internal/api"
max_pkgs="${QUICK_MAX_PKGS:-12}"
jobs="${QUICK_JOBS:-3}"

base_ref="${1:-${LEVELRAIL_QUICK_BASE:-}}"
if [ -z "$base_ref" ]; then
	best=""
	for r in $(git for-each-ref --format='%(refname:short)' 'refs/remotes/origin/release/*') origin/main; do
		mb="$(git merge-base HEAD "$r" 2>/dev/null)" || continue
		n="$(git rev-list --count "$mb..HEAD")"
		if [ -z "$best" ] || [ "$n" -lt "$best" ]; then
			base_ref="$r"
			best="$n"
		fi
	done
fi
merge_base="$(git merge-base HEAD "$base_ref" 2>/dev/null)" || {
	echo "FAIL base: no merge base with $base_ref (git fetch origin)"
	exit 2
}

changed="$({ git diff --name-only --diff-filter=ACMRD "$merge_base"; git ls-files --others --exclude-standard; } | sort -u)"
if [ -z "$changed" ]; then
	echo "PASS nothing changed vs $base_ref"
	exit 0
fi

log_dir="$(git rev-parse --absolute-git-dir)/quick-check"
rm -rf "$log_dir" && mkdir -p "$log_dir" || exit 2
results="$log_dir/results.txt"
: >"$results"
pids=()
record() { printf '%s\t%s\t%s\n' "$1" "$2" "$3" >>"$results"; }

# run_step <name> <dir> <cmd...>: bounded parallel slot, one log per step.
run_step() {
	local name="$1" dir="$2" slug
	shift 2
	slug="$(printf '%s' "$name" | tr -c 'A-Za-z0-9' '_')"
	while [ "${#pids[@]}" -ge "$jobs" ]; do
		wait "${pids[0]}"
		pids=("${pids[@]:1}")
	done
	( cd "$dir" && "$@" >"$log_dir/$slug.log" 2>&1; echo $? >"$log_dir/$slug.status" ) &
	pids+=("$!")
	record "$name" RUN "$slug"
}

# ---- Go ----
go_files="$(printf '%s\n' "$changed" | grep -E '\.go$' | grep -v -e '^tools/' -e '^kit/' -e '^integrations/' -e '^docs-local/' | while read -r f; do [ -f "$f" ] && echo "$f"; done)"
if [ -z "$go_files" ]; then
	record go SKIP "no Go files changed"
else
	direct="$(printf '%s\n' "$go_files" | xargs -n1 dirname | sort -u | while read -r d; do go list -e "./$d" 2>/dev/null; done | sort -u)"
	affected="$(printf '%s\n' "$changed" | scripts/affected-go-packages.sh --stdin 2>/dev/null)"
	scope="$direct"
	note="changed packages only"
	if [ "$affected" != "ALL" ] && [ -n "$affected" ]; then
		count="$(printf '%s\n' "$affected" | wc -l | tr -d ' ')"
		if [ "$count" -le "$max_pkgs" ]; then
			scope="$affected"
			note="$count packages incl. reverse deps"
		else
			# Guard packages (execgate and friends) are cheap and must still run.
			guards="$(grep -v '^#' scripts/ci-guards.txt | grep . | sed "s#^#$module/#" | grep -xF -f - <(printf '%s\n' "$affected") || true)"
			scope="$(printf '%s\n%s\n' "$direct" "$guards" | grep . | sort -u)"
			note="$count affected, over QUICK_MAX_PKGS=$max_pkgs: changed plus guards, CI runs the rest"
		fi
	fi
	other="$(printf '%s\n' "$scope" | grep -vxF "$api_pkg")"
	# shellcheck disable=SC2086
	run_step "go-vet ($note)" . go vet $scope
	if command -v golangci-lint >/dev/null; then
		lint_dirs="$(printf '%s\n' "$direct" | sed "s#^$module#.#")"
		# shellcheck disable=SC2086
		run_step go-lint . env GOLANGCI_LINT_CACHE="$(git rev-parse --absolute-git-dir)/golangci-lint-cache" golangci-lint run --new-from-rev="$merge_base" $lint_dirs
	else
		record go-lint SKIP "golangci-lint not installed"
	fi
	if [ -n "$other" ]; then
		# shellcheck disable=SC2086
		run_step go-test . go test -short -timeout 10m $other
	else
		record go-test SKIP "only internal/api in scope"
	fi
	if printf '%s\n' "$scope" | grep -qxF "$api_pkg"; then
		api_re="$(printf '%s\n' "$changed" | grep '^internal/api/.*_test\.go$' | while read -r f; do grep -ho '^func \(Test\|Fuzz\|Example\)[A-Za-z0-9_]*' "$f" 2>/dev/null | sed 's/^func //'; done | sort -u | paste -sd'|' -)"
		narrowed=""
		if [ -n "$other" ]; then
			# shellcheck disable=SC2086
			narrowed="$(scripts/affected-api-tests.sh $other 2>/dev/null || echo ALL)"
			[ "$narrowed" = ALL ] && narrowed=""
		fi
		api_re="${api_re:+$api_re|}$narrowed"
		api_re="${api_re%|}"
		if [ -n "$api_re" ]; then
			run_step go-test-api . scripts/test-fast.sh -r "^($api_re)\$"
		else
			record go-test-api SKIP "internal/api in scope but no test narrows to it: CI runs it (or scripts/test-fast.sh)"
		fi
	fi
fi

# ---- OpenAPI drift: only when a non-test internal/api source or the generator changed ----
if printf '%s\n' "$changed" | grep -E '^(internal/api/[^/]+\.go|scripts/gen-api-reference/.*|docs/api-reference\.md|docs/public/openapi\.json)$' | grep -qv '_test\.go$'; then
	gen_files="docs/api-reference.md internal/api/openapi_gen.go docs/public/openapi.json"
	# shellcheck disable=SC2086
	before="$(git diff -- $gen_files | shasum | cut -d' ' -f1)"
	if go run ./scripts/gen-api-reference >"$log_dir/openapi.log" 2>&1; then
		# shellcheck disable=SC2086
		after="$(git diff -- $gen_files | shasum | cut -d' ' -f1)"
		if [ "$before" = "$after" ]; then
			echo 0 >"$log_dir/openapi.status"
		else
			echo "OpenAPI drift: regenerated $gen_files, review and commit them" >>"$log_dir/openapi.log"
			echo 1 >"$log_dir/openapi.status"
		fi
	else
		echo 1 >"$log_dir/openapi.status"
	fi
	record openapi RUN openapi
else
	record openapi SKIP "no internal/api source or generator change"
fi

# ---- Web ----
web_changed="$(printf '%s\n' "$changed" | grep '^web/' || true)"
web_src="$(printf '%s\n' "$web_changed" | grep -E '^web/src/.*\.(ts|tsx)$' | grep -v routeTree.gen | while read -r f; do [ -f "$f" ] && echo "${f#web/}"; done)"
if [ -z "$web_changed" ]; then
	record web SKIP "no web files changed"
elif [ ! -d web/node_modules ]; then
	record web FAIL "web/node_modules missing (cd web && npm ci)"
else
	if scripts/ensure-route-tree.sh 2>"$log_dir/routetree.log"; then
		run_step web-tsc web npx tsc -b
	else
		record web-routetree FAIL "$(cat "$log_dir/routetree.log")"
	fi
	if [ -n "$web_src" ]; then
		# shellcheck disable=SC2086
		run_step web-eslint web npx eslint $web_src
		# shellcheck disable=SC2086
		run_step web-vitest web npx vitest related --run --passWithNoTests $web_src --reporter=dot
	else
		record web-vitest SKIP "no web/src TypeScript changed"
	fi
fi

for p in "${pids[@]}"; do wait "$p"; done

# ---- Report: failing lines only ----
fails=0 passes=0 skips=0
while IFS=$'\t' read -r name verdict detail; do
	case "$verdict" in
	SKIP) skips=$((skips + 1)); echo "SKIP $name ($detail)" ;;
	FAIL) fails=$((fails + 1)); echo "FAIL $name ($detail)" ;;
	RUN)
		status="$(cat "$log_dir/$detail.status" 2>/dev/null || echo 1)"
		if [ "$status" = 0 ]; then
			passes=$((passes + 1))
			echo "PASS $name"
		else
			fails=$((fails + 1))
			echo "FAIL $name (exit $status, full log: $log_dir/$detail.log)"
			grep -E -- '^(--- FAIL|FAIL|panic:)|_test\.go:[0-9]+|\.go:[0-9]+:[0-9]+|\.tsx?[(:][0-9]+|error|Error|drift' "$log_dir/$detail.log" | head -n 25 | sed 's/^/    /'
		fi
		;;
	esac
done <"$results"
echo "quick-check vs $base_ref: $passes passed, $fails failed, $skips skipped"
[ "$fails" -eq 0 ]
