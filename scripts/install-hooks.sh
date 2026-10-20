#!/bin/bash
# Installs a stub for each scripts/git-hooks/* into .git/hooks/ and symlinks
# CLAUDE.md into the current worktree. .git/hooks is shared across all
# worktrees of a repo (this repo never sets core.hooksPath). Each stub runs
# the hook script of whichever checkout fires it, so editing a hook takes
# effect at once with no reinstall, and removing a worktree leaves nothing
# dangling. Run this once per clone.
set -euo pipefail

repo_root="$(git rev-parse --show-toplevel)"
hooks_dir="$(git rev-parse --git-path hooks)"
mkdir -p "$hooks_dir"

for hook in "$repo_root"/scripts/git-hooks/*; do
	name="$(basename "$hook")"
	cat >"$hooks_dir/$name" <<STUB
#!/bin/bash
# Installed by scripts/install-hooks.sh: runs this checkout's own hook script.
hook="\$(git rev-parse --show-toplevel)/scripts/git-hooks/$name"
if [ ! -x "\$hook" ]; then
	echo "$name: \$hook not found, skipping" >&2
	exit 0
fi
exec "\$hook" "\$@"
STUB
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
