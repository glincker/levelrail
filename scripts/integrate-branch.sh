#!/usr/bin/env bash
# Merges finished feature branches into the current release branch, regenerates
# the generated files that always collide, verifies, pushes, removes the merged
# branch's worktree and restarts the local app.
#
# Usage (from the release worktree): scripts/integrate-branch.sh feat/a feat/b
# Background: scripts/integrate-branch.sh feat/a >"$HOME/levelrail-local-data/integrate.log" 2>&1 &
# Env: INTEGRATE_NO_PUSH=1 skips the push, INTEGRATE_NO_RESTART=1 skips the app restart.
# A conflict outside the known generated files aborts that merge and moves on.

set -uo pipefail

repo_root="$(git rev-parse --show-toplevel)"
cd "$repo_root"
release="$(git rev-parse --abbrev-ref HEAD)"
export GOLANGCI_LINT_CACHE="${GOLANGCI_LINT_CACHE:-$HOME/.cache/golangci-lint-orch}"
export LEVELRAIL_PUSH_PR_SIGNAL=off

[ $# -gt 0 ] || { echo "usage: $0 <branch> [branch...]" >&2; exit 2; }
case "$release" in release/*) ;; *) echo "integrate: refusing to run on '$release', expected release/*" >&2; exit 2 ;; esac
[ -z "$(git status --porcelain)" ] || { echo "integrate: working tree is dirty" >&2; exit 2; }

regen_api() { go run ./scripts/gen-api-reference >/dev/null 2>&1; }
regen_catalog() {
	go run ./scripts/gen-template-catalog-docs >/dev/null 2>&1 &&
		go run ./scripts/gen-template-gallery >/dev/null 2>&1
}

resolve_conflicts() {
	local f unresolved=""
	for f in $(git diff --name-only --diff-filter=U); do
		case "$f" in
		docs/api-reference.md | internal/api/openapi_gen.go | docs/public/openapi.json)
			git checkout --ours -- "$f" 2>/dev/null
			regen_api && git add "$f" || unresolved="$unresolved $f"
			;;
		docs/template-catalog.md | docs/.vitepress/data/templates.json)
			git checkout --ours -- "$f" 2>/dev/null
			regen_catalog && git add "$f" || unresolved="$unresolved $f"
			;;
		web/src/i18n/index.ts | web/src/i18n/resources.ts)
			# Both sides only append namespace lines, keep both.
			sed -i.bak -e '/^<<<<<<< /d' -e '/^=======$/d' -e '/^>>>>>>> /d' "$f" && rm -f "$f.bak"
			git add "$f"
			;;
		*) unresolved="$unresolved $f" ;;
		esac
	done
	[ -z "$unresolved" ] || { echo "integrate: unresolved:$unresolved" >&2; return 1; }
}

worktree_for_branch() {
	git worktree list --porcelain | awk -v b="refs/heads/$1" '
		/^worktree /{p=$2} /^branch /{if ($2==b) print p}'
}

merged=()
failed=()
for b in "$@"; do
	echo "integrate: merging $b"
	git fetch -q origin "$b" || { echo "integrate: cannot fetch $b" >&2; failed+=("$b"); continue; }
	if git merge --no-ff -q "origin/$b" -m "chore: merge $b into release ${release#release/}" >/dev/null 2>&1; then
		merged+=("$b")
		continue
	fi
	if resolve_conflicts && git commit -q -m "chore: merge $b into release ${release#release/}"; then
		merged+=("$b")
	else
		git merge --abort 2>/dev/null
		failed+=("$b")
	fi
done

[ ${#merged[@]} -gt 0 ] || { echo "integrate: nothing merged, failed: ${failed[*]}" >&2; exit 1; }

dups="$(ls internal/store/migrations | sed 's/_.*//' | sort | uniq -d)"
if [ -n "$dups" ]; then
	echo "integrate: DUPLICATE MIGRATION VERSIONS, not pushing: $dups" >&2
	exit 1
fi

echo "integrate: go build"
go build ./... || { echo "integrate: go build failed, not pushing" >&2; exit 1; }
if git diff --name-only "origin/$release...HEAD" | grep -q '^web/'; then
	echo "integrate: tsc -b"
	(cd web && npx tsc -b) || { echo "integrate: tsc failed, not pushing" >&2; exit 1; }
fi

if [ -z "${INTEGRATE_NO_PUSH:-}" ]; then
	git push origin "$release" || { echo "integrate: push failed" >&2; exit 1; }
fi

main_root="$(git worktree list --porcelain | awk '/^worktree /{print $2; exit}')"
for b in "${merged[@]}"; do
	wt="$(worktree_for_branch "$b")"
	if [ -n "$wt" ] && [ "$wt" != "$repo_root" ] && [ -z "$(git -C "$wt" status --porcelain)" ]; then
		git -C "$main_root" worktree unlock "$wt" 2>/dev/null
		git -C "$main_root" worktree remove --force "$wt" && echo "integrate: removed worktree $wt"
	fi
done

[ -n "${INTEGRATE_NO_RESTART:-}" ] || "$repo_root/scripts/dev-local.sh" restart

echo "integrate: merged ${merged[*]}"
[ ${#failed[@]} -eq 0 ] || { echo "integrate: FAILED (needs a human): ${failed[*]}" >&2; exit 1; }
