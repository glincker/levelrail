#!/bin/bash
# Installs scripts/git-hooks/* into .git/hooks/ and symlinks CLAUDE.md
# into the current worktree. .git/hooks is shared across all worktrees of
# a repo (this repo never sets core.hooksPath, so there's no per-worktree
# override), so re-running this after editing a hook updates every
# worktree at once. Run this once per clone, and again after editing a
# hook script.
set -euo pipefail

repo_root="$(git rev-parse --show-toplevel)"
hooks_dir="$(git rev-parse --git-path hooks)"
mkdir -p "$hooks_dir"

for hook in "$repo_root"/scripts/git-hooks/*; do
	name="$(basename "$hook")"
	cp "$hook" "$hooks_dir/$name"
	chmod +x "$hooks_dir/$name"
	echo "installed $name"
done

# CLAUDE.md is gitignored (no AI-authored artifacts in the repo itself),
# so `git worktree add` never brings it into a new worktree. Symlink it
# from the main worktree so a session started there can read it.
main_root="$(git rev-parse --path-format=absolute --git-common-dir)"
main_root="${main_root%/.git}"
if [ "$repo_root" != "$main_root" ] && [ -f "$main_root/CLAUDE.md" ] && [ ! -e "$repo_root/CLAUDE.md" ]; then
	ln -s "$main_root/CLAUDE.md" "$repo_root/CLAUDE.md"
	echo "symlinked CLAUDE.md from $main_root"
fi
