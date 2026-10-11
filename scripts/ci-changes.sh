#!/usr/bin/env bash
# Decides which ci.yml jobs have real work for a change, without a Go
# toolchain (the Go-level scoping happens in scripts/ci-go-plan.sh).
#
# Usage: scripts/ci-changes.sh <base> [head]   diff base..head
#        scripts/ci-changes.sh --full          everything
# Output: key=value lines for $GITHUB_OUTPUT, a human summary on stderr.
#
#   full        true when the CI pipeline itself changed, so run everything
#   go          any Go job has work (Build, vet and the test lanes)
#   go_full     skip import-graph scoping, test ./...
#   live_smoke  go_full came from pipeline files only: run the live suites in smoke mode
#   lint        none | changed | all
#   lint_dirs   package dirs for lint=changed
#   tools       the separate tools/ module changed
#   kit         the separate kit/ module changed (its own vet, lint, test lane)
#   web         none | docs (docs/ feeds the web build's help manifest) | full
#   vitest      none | changed | full
#   installer   install.sh end-to-end job
#   workflows   actionlint
set -euo pipefail

cd "$(git rev-parse --show-toplevel)"

full=false go=false go_full=false lint=none tools=false kit=false
dep_full=false
web=none vitest=none installer=false workflows=false
declare -A lint_dirs=()
reasons=()

set_full() {
	full=true
	reasons+=("$1")
}

if [ "${1:-}" = "--full" ]; then
	set_full "full run requested"
	dep_full=true
	files=()
else
	base="${1:?usage: ci-changes.sh <base> [head] | --full}"
	head="${2:-HEAD}"
	mapfile -t files < <(git diff --name-only --diff-filter=ACDMR "$base" "$head")
fi

for f in "${files[@]}"; do
	case "$f" in
	.github/workflows/ci.yml | scripts/ci-changes.sh | scripts/ci-lane-*.sh)
		set_full "CI pipeline changed: $f"
		;;
	scripts/ci-*.sh | scripts/affected-go-packages.sh | scripts/go-test-groups.sh | scripts/ci-guards.txt | \
		scripts/ci-heavy-packages.txt | scripts/check-coverage.sh | scripts/check-changed-file-coverage.sh | \
		scripts/merge-coverprofiles.sh | scripts/check-lane-results.sh | scripts/check-migration-versions.sh | \
		scripts/check-flaky-tests.sh | scripts/check-fanin.sh | scripts/fanin-budget.txt)
		# Go-side tooling only: re-run every Go lane, not web, installer or kit.
		go=true go_full=true
		reasons+=("Go pipeline script changed: $f")
		;;
	scripts/check-brand-strings.sh)
		go=true go_full=true kit=true
		reasons+=("brand check changed: $f")
		;;
	go.mod | go.sum)
		go=true go_full=true lint=all dep_full=true
		reasons+=("import graph can't scope: $f")
		;;
	kit/go.mod | kit/go.sum)
		go=true go_full=true kit=true dep_full=true
		reasons+=("import graph can't scope: $f")
		;;
	kit/*.md) ;;
	kit/*)
		go=true kit=true
		;;
	.golangci.yml)
		lint=all kit=true
		;;
	.github/flaky-tests.txt)
		go=true
		;;
	.github/workflows/*)
		workflows=true
		;;
	tools/*)
		go=true tools=true
		;;
	install.sh | install-cli.sh | scripts/test-install-sh.sh | scripts/test-install-cli.sh | packaging/*)
		installer=true
		;;
	web/*.go) ;;
	web/package.json | web/package-lock.json | web/vite.config.ts | web/vitest.config.ts | \
		web/tsconfig*.json | web/eslint.config.js | web/src/test/* | web/vite-plugins/*)
		web=full vitest=full
		;;
	web/*)
		web=full
		[ "$vitest" = full ] || vitest=changed
		;;
	docs/*.md)
		[ "$web" = full ] || web=docs
		;;
	esac
	case "$f" in
	*.go)
		case "$f" in tools/* | kit/*) continue ;; esac
		dir="$(dirname "$f")"
		[ -d "$dir" ] && lint_dirs["./$dir/"]=1
		;;
	esac
done

if [ "$full" = false ] && [ "${#files[@]}" -gt 0 ]; then
	seeds="$(printf '%s\n' "${files[@]}" | scripts/affected-go-packages.sh --stdin --seeds)"
	if [ "$seeds" = "ALL" ]; then
		go=true go_full=true
		# ALL caused only by pipeline files is not a dependency change.
		dep_seeds="$(printf '%s\n' "${files[@]}" | grep -vE '^(scripts/|\.github/|docs/)' | scripts/affected-go-packages.sh --stdin --seeds || true)"
		[ "$dep_seeds" = "ALL" ] && dep_full=true
	elif [ -n "$seeds" ]; then
		go=true
	fi
fi

if [ "$full" = true ]; then
	go=true go_full=true lint=all tools=true kit=true web=full vitest=full installer=true workflows=true
fi

if [ "$lint" = none ] && [ "${#lint_dirs[@]}" -gt 0 ]; then
	lint=changed
fi
lint_list=""
if [ "$lint" = changed ]; then
	lint_list="$(printf '%s\n' "${!lint_dirs[@]}" | sort | paste -sd' ' -)"
fi

cat <<EOF
full=$full
live_smoke=$([ "$dep_full" = false ] && { [ "$full" = true ] || [ "$go_full" = true ]; } && echo true || echo false)
go=$go
go_full=$go_full
lint=$lint
lint_dirs=$lint_list
tools=$tools
kit=$kit
web=$web
vitest=$vitest
installer=$installer
workflows=$workflows
EOF

{
	echo "changed files: ${#files[@]}"
	for r in "${reasons[@]}"; do echo "  $r"; done
	echo "go=$go go_full=$go_full lint=$lint tools=$tools kit=$kit web=$web vitest=$vitest installer=$installer workflows=$workflows"
} >&2
