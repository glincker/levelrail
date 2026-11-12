#!/usr/bin/env bash
# Maps changed files to the fast repo-wide guards that cover them and runs only
# those, so a push does not learn about a guard from CI.
# Usage: scripts/preflight.sh [--base <ref>] [--all] [--plan] [--stdin]
#   --base <ref>  diff against this ref (default: origin/release/v0.9.0, then origin/main)
#   --all         run every guard regardless of the diff
#   --plan        print the guards the files select and exit (no run)
#   --stdin       read the changed-file list from stdin instead of git
# Env: PREFLIGHT_BUDGET_SECONDS (default 90), PREFLIGHT_DEFAULT_BASE.
set -uo pipefail

# Mapping table: "<extended regex on a changed path>|<guard id>". One line per rule.
# A new guard is one line here plus one case in guard_cmd below.
RULES=(
	'^(cmd/levelrail-cli/|internal/clitree/)|clitree'
	'^(internal/catalog/|web/src/lib/templateLogos\.ts$|web/src/lib/templateLogos\.test\.ts$|stacks/)|catalog'
	'^(internal/catalog/|web/src/lib/templateLogos\.ts$|stacks/)|logos'
	'^(internal/catalog/|stacks/|scripts/validate-templates/)|validate-templates'
	'^(internal/api/.*routes.*\.go|internal/api/openapi_gen\.go|docs/|internal/catalog/|stacks/|internal/runtimesettings/|scripts/gen-|scripts/docs-fragments/|scripts/code-gen-check\.sh|web/src/routes/)|codegen'
	'(\.go$|^go\.mod$|^go\.sum$)|fanin'
	'(\.go$|^go\.mod$|^go\.sum$)|execgate'
	'^internal/runtimesettings/|runtimesettings'
	'^web/src/(locales|i18n)/|locales'
	'^web/src/(locales|i18n)/|localecheck'
	'^docs/|docs'
)
# Guards that run on every invocation.
ALWAYS=(dashes)
# Guards that rewrite tracked files run alone, after the parallel group.
SERIAL=(codegen)

# Prints the shell command of one guard. Exit 3 from it means "skipped, with a hint".
guard_cmd() {
	case "$1" in
	clitree) echo "go test ./cmd/levelrail-cli -run 'CommandTree|HelpText|Console|JSONCoverage'" ;;
	catalog) echo "go test ./internal/catalog -short" ;;
	logos) echo "if [ -d web/node_modules ]; then cd web && node_modules/.bin/vitest run src/lib/templateLogos.test.ts; else echo 'hint: web/node_modules missing, run scripts/dev-bootstrap.sh to check template logos'; exit 3; fi" ;;
	validate-templates) echo "go run ./scripts/validate-templates" ;;
	codegen) echo "scripts/code-gen-check.sh" ;;
	fanin) echo "scripts/check-fanin.sh" ;;
	execgate) echo "go test ./test/execgate" ;;
	runtimesettings) echo "go test ./internal/runtimesettings" ;;
	locales) echo "if [ -d web/node_modules ]; then cd web && node_modules/.bin/vitest run src/i18n/localeParity.test.ts src/i18n/locales.test.ts; else echo 'hint: web/node_modules missing, run scripts/dev-bootstrap.sh to check locale parity'; exit 3; fi" ;;
	docs) echo "bash scripts/docs-check.sh --links-only" ;;
	localecheck) echo "scripts/check-locales.sh" ;;
	dashes) echo "scripts/check-dashes.sh" ;;
	*) echo "echo unknown guard $1; exit 2" ;;
	esac
}

# Reads changed paths on stdin, prints each selected guard id once, in table order.
select_guards() {
	local files rule re id
	files="$(cat)"
	for id in "${ALWAYS[@]}"; do echo "$id"; done
	for rule in "${RULES[@]}"; do
		re="${rule%|*}"
		id="${rule##*|}"
		if printf '%s\n' "$files" | grep -qE "$re"; then echo "$id"; fi
	done | awk '!seen[$0]++'
}

all_guards() {
	local rule
	for rule in "${RULES[@]}"; do echo "${rule##*|}"; done
	printf '%s\n' "${ALWAYS[@]}"
}

is_serial() {
	local s
	for s in "${SERIAL[@]}"; do [ "$s" = "$1" ] && return 0; done
	return 1
}

base=""
run_all=0
plan_only=0
from_stdin=0
while [ $# -gt 0 ]; do
	case "$1" in
	--base) base="${2:-}"; shift 2 ;;
	--all) run_all=1; shift ;;
	--plan) plan_only=1; shift ;;
	--stdin) from_stdin=1; shift ;;
	*) echo "preflight: unknown argument $1" >&2; exit 2 ;;
	esac
done

cd "$(git rev-parse --show-toplevel)" || exit 2
export GOFLAGS="${GOFLAGS:--buildvcs=false}"

if [ "$from_stdin" -eq 1 ]; then
	changed="$(cat)"
else
	if [ -z "$base" ]; then
		for r in "${PREFLIGHT_DEFAULT_BASE:-origin/release/v0.9.0}" origin/main; do
			if git rev-parse --verify --quiet "$r" >/dev/null; then base="$r"; break; fi
		done
	fi
	changed="$(
		{
			if [ -n "$base" ] && mb="$(git merge-base HEAD "$base" 2>/dev/null)"; then git diff --name-only "$mb...HEAD"; fi
			git diff --name-only --cached
			git diff --name-only
		} | sort -u
	)"
fi

if [ "$run_all" -eq 1 ]; then
	guards="$(all_guards | awk '!seen[$0]++')"
else
	guards="$(printf '%s\n' "$changed" | select_guards)"
fi

if [ "$plan_only" -eq 1 ]; then
	printf '%s\n' "$guards"
	exit 0
fi

budget="${PREFLIGHT_BUDGET_SECONDS:-90}"
tmp="$(mktemp -d -t levelrail-preflight.XXXXXX)"
trap 'rm -rf "$tmp"' EXIT
start=$SECONDS
nfiles=0
[ -n "$changed" ] && nfiles="$(printf '%s\n' "$changed" | wc -l | tr -d ' ')"
echo "preflight: $nfiles changed file(s), base ${base:-none}, budget ${budget}s"

# Runs one guard, recording exit code and seconds in $tmp/<id>.rc.
run_guard() {
	local id="$1" t0 rc
	t0=$SECONDS
	bash -c "$(guard_cmd "$id")" >"$tmp/$id.log" 2>&1
	rc=$?
	echo "$rc $((SECONDS - t0))" >"$tmp/$id.rc"
}

report() {
	local id="$1" rc secs
	read -r rc secs <"$tmp/$id.rc"
	case "$rc" in
	0) printf 'preflight: ran     pass  %-20s %ss\n' "$id" "$secs" ;;
	3) printf 'preflight: skipped hint  %-20s %ss\n' "$id" "$secs"; sed 's/^/             /' "$tmp/$id.log" ;;
	*)
		printf 'preflight: ran     FAIL  %-20s %ss\n' "$id" "$secs"
		tail -n 40 "$tmp/$id.log" | sed 's/^/             /'
		failed=1
		;;
	esac
}

failed=0
skipped_list=""
parallel=""
serial=""
for id in $guards; do
	if is_serial "$id"; then serial="$serial $id"; else parallel="$parallel $id"; fi
done

for id in $parallel; do run_guard "$id" & done
wait
for id in $parallel; do report "$id"; done

for id in $serial; do
	if [ $((SECONDS - start)) -gt "$budget" ]; then
		printf 'preflight: skipped over-budget %-14s (CI will run it)\n' "$id"
		skipped_list="$skipped_list $id"
		continue
	fi
	run_guard "$id"
	report "$id"
done

# Guards the diff did not select are listed so a skip is never silent.
notsel=""
for id in $(all_guards | awk '!seen[$0]++'); do
	printf '%s\n' "$guards" | grep -qxF "$id" || notsel="$notsel $id"
done
[ -n "$notsel" ] && echo "preflight: not triggered by this diff:$notsel"

total=$((SECONDS - start))
if [ "$total" -gt "$budget" ]; then
	echo "preflight: took ${total}s, over the ${budget}s budget. Skipped for time:${skipped_list:- none}. Slow guards are listed above."
fi
if [ "$failed" -ne 0 ]; then
	echo "preflight: FAILED in ${total}s"
	exit 1
fi
echo "preflight: ok in ${total}s"
