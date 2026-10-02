#!/usr/bin/env bash
# Decides which ci.yml jobs have real work for a change, without a Go
# toolchain (the Go-level scoping happens in scripts/ci-go-plan.sh).
#
# Usage: scripts/ci-changes.sh <base> [head]   diff base..head (default HEAD)
#        scripts/ci-changes.sh --full          everything (push to main)
# Output: key=value lines for $GITHUB_OUTPUT, a human summary on stderr.
#
#   full        true when the CI pipeline itself changed, so run everything
#   go          any Go job has work (Build, vet and the test lanes)
#   go_full     skip import-graph scoping, test ./...
#   lint        none | changed | all
#   lint_dirs   package dirs for lint=changed
#   tools       the separate tools/ module changed
#   web         none | docs (docs/ feeds the web build's help manifest) | full
#   vitest      none | changed | full
#   installer   install.sh end-to-end job
#   workflows   actionlint
set -euo pipefail

cd "$(git rev-parse --show-toplevel)"

full=false go=false go_full=false lint=none tools=false
web=none vitest=none installer=false workflows=false
declare -A lint_dirs=()
reasons=()

set_full() {
	full=true
	reasons+=("$1")
}

if [ "${1:-}" = "--full" ]; then
	set_full "full run requested"
	files=()
else
	base="${1:?usage: ci-changes.sh <base> [head] | --full}"
	head="${2:-HEAD}"
	mapfile -t files < <(git diff --name-only --diff-filter=ACDMR "$base" "$head")
fi

for f in "${files[@]}"; do
	case "$f" in
	.github/workflows/ci.yml | scripts/ci-*.sh | scripts/affected-go-packages.sh | scripts/go-test-groups.sh | \
		scripts/check-lane-results.sh | scripts/check-coverage.sh | scripts/check-changed-file-coverage.sh | \
		scripts/merge-coverprofiles.sh | scripts/check-migration-versions.sh | scripts/check-flaky-tests.sh)
		set_full "CI pipeline changed: $f"
		;;
	go.mod | go.sum | internal/store/migrations/*.sql)
		go=true go_full=true lint=all
		reasons+=("import graph can't scope: $f")
		;;
	.golangci.yml)
		lint=all
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
		case "$f" in tools/*) continue ;; esac
		dir="$(dirname "$f")"
		[ -d "$dir" ] && lint_dirs["./$dir/"]=1
		;;
	esac
done

if [ "$full" = false ] && [ "${#files[@]}" -gt 0 ]; then
	seeds="$(printf '%s\n' "${files[@]}" | scripts/affected-go-packages.sh --stdin --seeds)"
	if [ "$seeds" = "ALL" ]; then
		go=true go_full=true
	elif [ -n "$seeds" ]; then
		go=true
	fi
fi

if [ "$full" = true ]; then
	go=true go_full=true lint=all tools=true web=full vitest=full installer=true workflows=true
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
go=$go
go_full=$go_full
lint=$lint
lint_dirs=$lint_list
tools=$tools
web=$web
vitest=$vitest
installer=$installer
workflows=$workflows
EOF

{
	echo "changed files: ${#files[@]}"
	for r in "${reasons[@]}"; do echo "  $r"; done
	echo "go=$go go_full=$go_full lint=$lint tools=$tools web=$web vitest=$vitest installer=$installer workflows=$workflows"
} >&2
