#!/usr/bin/env bash
# Merges finished feature branches into the current release branch, regenerates
# the generated files that always collide, verifies, pushes, removes the merged
# branch's worktree and restarts the local app.
#
# Usage (from the release worktree): scripts/integrate-branch.sh feat/a feat/b
# Background: scripts/integrate-branch.sh feat/a >"$HOME/levelrail-local-data/integrate.log" 2>&1 &
# Env: INTEGRATE_NO_PUSH=1 skips the push, INTEGRATE_NO_RESTART=1 skips the app restart.
# A conflict outside the known generated files aborts that merge and moves on.
# INTEGRATE_LOCAL=1 merges local branch refs (agent branches that are not pushed).
# INTEGRATE_OLD_BASE (default origin/release/v0.8) names a squash-merged line to strip.

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
		docs/template-catalog.md)
			git checkout --ours -- "$f" 2>/dev/null
			regen_catalog && git add "$f" || unresolved="$unresolved $f"
			;;
		web/src/components/BrandIcon.tsx | web/src/i18n/index.ts | web/src/i18n/resources.ts)
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

old_base="${INTEGRATE_OLD_BASE:-origin/release/v0.8}"
merged=()
failed=()
for b in "$@"; do
	echo "integrate: merging $b"
	if [ -n "${INTEGRATE_LOCAL:-}" ]; then
		git rev-parse -q --verify "refs/heads/$b" >/dev/null || { echo "integrate: no local branch $b" >&2; failed+=("$b"); continue; }
		src="refs/heads/$b"
	else
		git fetch -q origin "$b" || { echo "integrate: cannot fetch $b" >&2; failed+=("$b"); continue; }
		src="origin/$b"
	fi
	# A branch cut from the previous release line drags that line's commits back in
	# once it was squash merged, so replay only its own commits onto this release.
	if git merge-base --is-ancestor "$old_base" "$src" 2>/dev/null &&
		! git merge-base --is-ancestor "$old_base" HEAD 2>/dev/null; then
		tmp="integrate-tmp-$$"
		git checkout -q -b "$tmp" HEAD &&
			git cherry-pick $(git rev-list --reverse --no-merges "$old_base..$src") >/dev/null 2>&1
		if [ $? -ne 0 ]; then
			git cherry-pick --abort 2>/dev/null
			git checkout -q "$release" && git branch -D -q "$tmp"
			echo "integrate: could not replay $b onto $release" >&2
			failed+=("$b")
			continue
		fi
		git checkout -q "$release"
		src="$tmp"
	fi
	if git merge --no-ff -q "$src" -m "chore: merge $b into release ${release#release/}" >/dev/null 2>&1; then
		merged+=("$b")
		continue
	fi
	if resolve_conflicts && git commit -q -m "chore: merge $b into release ${release#release/}"; then
		merged+=("$b")
	else
		git merge --abort 2>/dev/null
		failed+=("$b")
	fi
	git branch -D -q "integrate-tmp-$$" 2>/dev/null
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
