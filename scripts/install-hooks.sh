#!/bin/bash
# Installs scripts/git-hooks/* into .git/hooks/. Deliberately copies
# files rather than setting core.hooksPath: .git/hooks is per-clone and
# untracked either way, and this repo's own policy is never to touch git
# config. Run this once per clone or worktree.
set -euo pipefail

repo_root="$(git rev-parse --show-toplevel)"
# --git-path resolves core.hooksPath and, in a worktree, the per-worktree
# hooks dir under the common .git dir, which doesn't exist until a hook
# is installed into it.
hooks_dir="$(git rev-parse --git-path hooks)"
mkdir -p "$hooks_dir"

for hook in "$repo_root"/scripts/git-hooks/*; do
	name="$(basename "$hook")"
	cp "$hook" "$hooks_dir/$name"
	chmod +x "$hooks_dir/$name"
	echo "installed $name"
done
