#!/usr/bin/env bash
# Runs the smallest useful local test set for the current diff: only the tests
# in changed test files plus the foo_test.go that matches each changed foo.go.
# Never whole packages, never dependents (CI and nightly cover those).
#
# Usage: scripts/test-slice.sh [--list] [base-ref]
# Base:  argument, else $LEVELRAIL_PUSH_BASE, else the closest origin/release/*
#        merge-base (same idea as the pre-push hook), else origin/main.
# Env:   TEST_SLICE_RACE=1       add -race (off by default)
#        TEST_SLICE_GUARDS_FILE  guard list (default scripts/ci-guards.txt)
#        TEST_SLICE_FAIL_LINES   max failure output lines shown (default 80)
#        TEST_SLICE_NODE_PATH    node bin dir for vitest (default /opt/homebrew/opt/node@22/bin)
# Known under-selection: a changed helper used only by other files' tests,
# tests that never share the changed file's name, and cross-package callers.
# Use scripts/affected-tests.sh when you want dependents too.

set -eo pipefail

cd "$(git rev-parse --show-toplevel)"

LIST=0
BASE=""
for a in "$@"; do
	case "$a" in
	--list) LIST=1 ;;
	*) BASE="$a" ;;
	esac
done

GUARDS_FILE="${TEST_SLICE_GUARDS_FILE:-scripts/ci-guards.txt}"
FAIL_LINES="${TEST_SLICE_FAIL_LINES:-80}"
NODE_PATH_DIR="${TEST_SLICE_NODE_PATH:-/opt/homebrew/opt/node@22/bin}"

[ -n "$BASE" ] || BASE="${LEVELRAIL_PUSH_BASE:-}"
if [ -z "$BASE" ]; then
	best=""
	best_n=999999
	for r in $(git for-each-ref --format='%(refname:short)' 'refs/remotes/origin/release/*') origin/main; do
		mb="$(git merge-base HEAD "$r" 2>/dev/null)" || continue
		n="$(git rev-list --count "$mb..HEAD" 2>/dev/null || echo 999999)"
		if [ "$n" -lt "$best_n" ]; then
			best="$r"
			best_n="$n"
		fi
	done
	BASE="${best:-origin/main}"
fi
echo "test-slice: base $BASE"
git rev-parse --verify --quiet "$BASE" >/dev/null || {
	echo "test-slice: unknown base ref $BASE" >&2
	exit 2
}

TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

# Committed diff plus staged, unstaged and untracked so it works pre-commit.
{
	git diff --name-only --diff-filter=ACMR "$BASE...HEAD" 2>/dev/null || true
	git diff --name-only --diff-filter=ACMR HEAD 2>/dev/null || true
	git ls-files --others --exclude-standard 2>/dev/null || true
} | sort -u | while read -r f; do [ -f "$f" ] && echo "$f"; done >"$TMP/changed" || true

grep -E '\.go$' "$TMP/changed" | grep -vE '(^|/)(testdata|vendor)/' >"$TMP/gofiles" || true
grep -vE '_test\.go$' "$TMP/gofiles" >"$TMP/srcfiles" || true

mod_root() {
	d="$1"
	while [ "$d" != "." ] && [ ! -f "$d/go.mod" ]; do d="$(dirname "$d")"; done
	echo "$d"
}

test_names() {
	grep -hE '^func (Test|Fuzz)[A-Za-z0-9_]*\(' "$@" 2>/dev/null | sed -E 's/^func ((Test|Fuzz)[A-Za-z0-9_]*)\(.*/\1/' | grep -vx 'TestMain' | sort -u || true
}

# plan lines: modroot<TAB>pkgrel<TAB>names (space separated, may be empty)
: >"$TMP/plan"
while read -r f; do dirname "$f"; done <"$TMP/gofiles" | sort -u | while read -r dir; do
	tf=""
	while read -r f; do
		[ "$(dirname "$f")" = "$dir" ] && tf="$tf $f"
	done <"$TMP/gofiles"
	files=""
	for f in $tf; do
		case "$f" in
		*_test.go) files="$files $f" ;;
		*)
			cand="${f%.go}_test.go"
			[ -f "$cand" ] && files="$files $cand"
			;;
		esac
	done
	names=""
	if [ -n "$files" ]; then
		# shellcheck disable=SC2086
		names="$(test_names $files | tr '\n' ' ')"
	fi
	mr="$(mod_root "$dir")"
	rel="$dir"
	[ "$mr" != "." ] && rel="${dir#"$mr"/}"
	[ "$rel" = "$dir" ] && [ "$mr" != "." ] && rel="."
	printf '%s\t%s\t%s\n' "$mr" "$rel" "$names" >>"$TMP/plan"
done

# Whole-tree guards, only when production Go code changed.
GUARD_PKGS=""
if [ -s "$TMP/srcfiles" ] && [ -f "$GUARDS_FILE" ]; then
	GUARD_PKGS="$(grep -vE '^\s*(#|$)' "$GUARDS_FILE" || true)"
fi

echo "test-slice: plan"
while IFS="$(printf '\t')" read -r mr rel names; do
	[ -n "$mr" ] || continue
	n="$(echo "$names" | wc -w | tr -d ' ')"
	if [ "$n" -gt 0 ]; then
		echo "  $mr:$rel  $n tests"
	else
		echo "  $mr:$rel  no mapped tests (compile check only)"
	fi
done <"$TMP/plan"
for g in $GUARD_PKGS; do echo "  .:$g  guard (all tests)"; done

grep -E '^web/src/.*\.(ts|tsx)$' "$TMP/changed" | grep -v 'routeTree.gen' | sed 's#^web/##' >"$TMP/webfiles" || true
WEB_CMD=""
if [ -s "$TMP/webfiles" ]; then
	WEB_CMD="cd web && PATH=$NODE_PATH_DIR:\$PATH npx vitest related --run $(tr '\n' ' ' <"$TMP/webfiles")"
	echo "  web: $WEB_CMD"
fi

if [ "$LIST" = 1 ]; then
	exit 0
fi

RACE=""
[ "${TEST_SLICE_RACE:-0}" = 1 ] && RACE="-race"
FAILED=0

# Runs one go test invocation in a module and prints one line per package.
run_group() {
	mr="$1"
	label="$2"
	shift 2
	out="$TMP/out.$$.$RANDOM"
	rc=0
	(cd "$mr" && go test -short $RACE "$@") >"$out" 2>&1 || rc=$?
	if [ "$rc" -ne 0 ]; then FAILED=1; fi
	while IFS="$(printf '\t')" read -r st pkg tm _; do
		case "$st" in
		ok*) echo "  ok    $pkg  $label  $tm" ;;
		FAIL*) [ -n "$pkg" ] && echo "  FAIL  $pkg  $label  $tm" ;;
		esac
	done <"$out"
	if [ "$rc" -ne 0 ]; then
		grep -vE '^(ok|\?)[[:space:]]' "$out" | head -n "$FAIL_LINES"
	fi
}

mods="$(cut -f1 "$TMP/plan" | sort -u)"
for mr in $mods; do
	mapped_pkgs=""
	unmapped_pkgs=""
	union=""
	while IFS="$(printf '\t')" read -r m rel names; do
		[ "$m" = "$mr" ] || continue
		p="./$rel"
		[ "$rel" = "." ] && p="."
		if [ -n "$names" ]; then
			mapped_pkgs="$mapped_pkgs $p"
			union="$union $names"
		else
			unmapped_pkgs="$unmapped_pkgs $p"
		fi
	done <"$TMP/plan"
	if [ -n "$mapped_pkgs" ]; then
		re="^($(echo "$union" | tr ' ' '\n' | grep -v '^$' | sort -u | tr '\n' '|' | sed 's/|$//'))\$"
		# shellcheck disable=SC2086
		run_group "$mr" "selected" -run "$re" $mapped_pkgs
	fi
	if [ -n "$unmapped_pkgs" ]; then
		echo "  no mapped tests, compile check:$unmapped_pkgs"
		# shellcheck disable=SC2086
		run_group "$mr" "compile-only" -run '^$' $unmapped_pkgs
	fi
done

for g in $GUARD_PKGS; do
	run_group . "guard" "./$g"
done

if [ -n "$WEB_CMD" ]; then
	if [ -d web/node_modules ]; then
		echo "  web: running vitest related"
		# shellcheck disable=SC2046
		(cd web && PATH="$NODE_PATH_DIR:$PATH" npx vitest related --run $(cat "$TMP/webfiles")) || FAILED=1
	else
		echo "  web: node_modules missing, skipped; run the command above after npm ci"
	fi
fi

if [ "$FAILED" -ne 0 ]; then
	echo "test-slice: FAIL"
	exit 1
fi
echo "test-slice: pass"
