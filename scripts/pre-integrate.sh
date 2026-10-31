#!/usr/bin/env bash
# One local gate to run before a branch is squash-merged into the release branch
# or the release PR is opened. Stops at the first failure, always prints a summary.
# Usage: scripts/pre-integrate.sh [--web] [--base REF]
# Base:  --base, else $LEVELRAIL_PUSH_BASE, else the closest origin/release/* merge-base.
# Env:   PRE_INTEGRATE_NODE_PATH  node bin dir (default /opt/homebrew/opt/node@22/bin)
#        PRE_INTEGRATE_NODE_HEAP  MB for tsc (default 8192)
#        PRE_INTEGRATE_TAIL       failure output lines shown (default 40)
#        PRE_INTEGRATE_MIGRATION_STRICT=1  fail on a number used by another remote branch
# Works on bash 3.2 (no mapfile, no associative arrays).
set -uo pipefail
cd "$(git rev-parse --show-toplevel)" || exit 2
# Shared Go build cache across worktrees: trimpath keeps the package dir out of action IDs.
case " ${GOFLAGS:-} " in *" -trimpath "*) ;; *) export GOFLAGS="${GOFLAGS:+$GOFLAGS }-trimpath" ;; esac

web=0
base=""
while [ $# -gt 0 ]; do
	case "$1" in
	--web) web=1 ;;
	--base) shift; base="${1:-}" ;;
	*) echo "usage: $0 [--web] [--base REF]" >&2; exit 2 ;;
	esac
	shift
done

node_path="${PRE_INTEGRATE_NODE_PATH:-/opt/homebrew/opt/node@22/bin}"
node_heap="${PRE_INTEGRATE_NODE_HEAP:-8192}"
tail_n="${PRE_INTEGRATE_TAIL:-40}"
[ -d "$node_path" ] && export PATH="$node_path:$PATH"

[ -n "$base" ] || base="${LEVELRAIL_PUSH_BASE:-}"
if [ -z "$base" ]; then
	best_n=""
	for r in $(git for-each-ref --format='%(refname:short)' 'refs/remotes/origin/release/*') origin/main; do
		mb="$(git merge-base HEAD "$r" 2>/dev/null)" || continue
		n="$(git rev-list --count "$mb..HEAD")"
		if [ -z "$best_n" ] || [ "$n" -lt "$best_n" ]; then
			base="$r"
			best_n="$n"
		fi
	done
	base="${base:-origin/main}"
fi
mergebase="$(git merge-base HEAD "$base" 2>/dev/null || echo "$base")"

# Changes since the merge-base, committed or not, plus new untracked files.
changed="$({ git diff --name-only --diff-filter=ACMR "$mergebase"; git ls-files -o --exclude-standard; } | sort -u)"
changed_go="$(printf '%s\n' "$changed" | grep -E '\.go$' || true)"
export CHANGED_GO="$changed_go" NODE_HEAP="$node_heap" MIGRATION_STRICT="${PRE_INTEGRATE_MIGRATION_STRICT:-0}"
has() { printf '%s\n' "$changed" | grep -Eq "$1"; }

log="$(mktemp)"
summary=""
failed=""
trap 'rm -f "$log"' EXIT

add() { summary="$summary$(printf '%-40s %-6s %s' "$1" "$2" "$3")
"; }

# step <label> <command string>; skipped once something has failed.
step() {
	local label="$1" cmd="$2" t0 rc
	if [ -n "$failed" ]; then add "$label" "-" "not run"; return; fi
	echo ">> $label"
	t0=$(date +%s)
	bash -c "$cmd" >"$log" 2>&1
	rc=$?
	if [ $rc -eq 0 ]; then
		add "$label" "pass" "$(( $(date +%s) - t0 ))s"
	else
		add "$label" "FAIL" "$(( $(date +%s) - t0 ))s"
		failed="$label"
		tail -n "$tail_n" "$log"
	fi
}
skip() { add "$1" "skip" "$2"; }

echo "pre-integrate: base $base (merge-base ${mergebase:0:9}), $(printf '%s\n' "$changed" | grep -c . || true) changed files"

step_if() { # <condition> <label> <skip reason> <command>
	if [ "$1" = 1 ]; then step "$2" "$4"; else skip "$2" "$3"; fi
}

go_changed=0; [ -n "$changed_go" ] && go_changed=1
locales=0; has '^web/src/locales/' && locales=1
api=0; has '^internal/api/|openapi|routes?[/._]' && api=1

step_if $go_changed "gofmt (changed files)" "no Go changes" \
	'out=$(printf "%s\n" "$CHANGED_GO" | while read -r f; do [ -f "$f" ] && echo "$f"; done | xargs gofmt -l); [ -z "$out" ] || { echo "not gofmt-clean:"; echo "$out"; exit 1; }'
step "code-gen-check" 'scripts/code-gen-check.sh'
step "migration versions + remotes" 'scripts/check-migration-versions.sh && MIGRATION_REMOTE_STRICT="$MIGRATION_STRICT" scripts/check-migration-versions.sh --against-remotes'
step "fan-in budget" 'scripts/check-fanin.sh'
step "dash sweep" 'scripts/check-dashes.sh'
step_if $locales "i18n parity" "no locale changes" 'cd web && npx vitest run src/i18n/localeParity.test.ts'
step_if $api "API guards (go test ./internal/api)" "no api/route/openapi changes" \
	"go test ./internal/api -run 'TestAPIStructsDoNotSerializeSecretLookingFields|TestRESTMutatingRoutesAreAudited|TestAuthzMatrix|TestTeamAuthzMatrix|OpenAPI|TestRoute'"
step_if $api "CLI guards (go test ./cmd/levelrail-cli)" "no api/route/openapi changes" \
	"go test ./cmd/levelrail-cli -run 'TestCommandTree|TestHelpTextCoversEveryFlag'"
step_if $go_changed "go vet (changed packages)" "no Go changes" \
	'pk=$(printf "%s\n" "$CHANGED_GO" | while read -r f; do [ -f "$f" ] && dirname "$f"; done | sort -u | sed "s#^#./#"); [ -z "$pk" ] || go vet $pk'
step_if $web "tsc -b" "needs --web" 'cd web && NODE_OPTIONS=--max-old-space-size=$NODE_HEAP npx tsc -b'
step_if $web "bundle size" "needs --web" \
	'cd web && { BUNDLE_MANIFEST=true npx vite build >/dev/null || exit 1; }; o=$(node scripts/check-bundle-size.js 2>&1); rc=$?; r=$(node scripts/bundle-report.js 2>&1); printf "%s\n%s\n" "$o" "$r"; [ $rc -eq 0 ] && ! printf "%s\n%s\n" "$o" "$r" | grep -q FAIL'

echo
echo "pre-integrate summary"
printf '%s' "$summary"
if [ -n "$failed" ]; then echo "RESULT: FAILED at '$failed'"; exit 1; fi
echo "RESULT: ok"
