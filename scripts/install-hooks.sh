#!/bin/bash
# Installs scripts/git-hooks/* into .git/hooks/. Deliberately copies
# files rather than setting core.hooksPath: .git/hooks is per-clone and
# untracked either way, and this repo's own policy is never to touch git
# config. Run this once per clone or worktree.
set -euo pipefail

repo_root="$(git rev-parse --show-toplevel)"
git_dir="$(git rev-parse --git-dir)"

for hook in "$repo_root"/scripts/git-hooks/*; do
	name="$(basename "$hook")"
	cp "$hook" "$git_dir/hooks/$name"
	chmod +x "$git_dir/hooks/$name"
	echo "installed $name"
done
