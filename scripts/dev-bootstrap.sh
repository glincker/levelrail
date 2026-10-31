#!/bin/bash
# Sets up web/node_modules in a worktree from one shared golden copy, built
# once per package-lock hash. Clones copy-on-write where the filesystem
# allows; never symlinks into another worktree.
#   scripts/dev-bootstrap.sh          set up web/node_modules
#   scripts/dev-bootstrap.sh --prune  drop stale golden copies
# Env: LEVELRAIL_GOLDEN_DIR (cache root), LEVELRAIL_GOLDEN_PRUNE_DAYS (14),
# LEVELRAIL_GOLDEN_LOCK_WAIT (seconds to wait on another builder, 900).
set -euo pipefail

repo_root="$(git -C "$(dirname "$0")/.." rev-parse --show-toplevel)"
web="$repo_root/web"
root="${LEVELRAIL_GOLDEN_DIR:-${XDG_CACHE_HOME:-$HOME/.cache}/levelrail/node_modules}"
prune_days="${LEVELRAIL_GOLDEN_PRUNE_DAYS:-14}"
lock_wait="${LEVELRAIL_GOLDEN_LOCK_WAIT:-900}"
stamp_name=".levelrail-lock-hash"

if [ ! -f "$web/package-lock.json" ]; then
	echo "dev-bootstrap: no web/package-lock.json, nothing to do"
	exit 0
fi

sha() {
	if command -v shasum >/dev/null 2>&1; then shasum -a 256; else sha256sum; fi
}
hash="$(cat "$web/package-lock.json" "$web/package.json" 2>/dev/null | sha | cut -c1-16)"
golden="$root/$hash"

if [ "${1:-}" = "--prune" ]; then
	n=0
	if [ -d "$root" ]; then
		while IFS= read -r d; do
			name="$(basename "$d")"
			case "$name" in
			*[!0-9a-f]* | "") continue ;;
			esac
			[ "${#name}" -eq 16 ] || continue
			[ "$name" = "$hash" ] && continue
			rm -rf "$d"
			n=$((n + 1))
		done < <(find "$root" -mindepth 1 -maxdepth 1 -type d -mtime "+$prune_days")
	fi
	echo "dev-bootstrap: pruned $n golden copies older than $prune_days days"
	exit 0
fi

if [ -f "$web/node_modules/$stamp_name" ] && [ "$(cat "$web/node_modules/$stamp_name")" = "$hash" ]; then
	echo "dev-bootstrap: web/node_modules already current ($hash)"
	exit 0
fi

build_golden() {
	local lock="$root/.lock-$hash" tmp="$root/.build-$hash" waited=0
	mkdir -p "$root"
	until mkdir "$lock" 2>/dev/null; do
		[ -d "$golden" ] && return 0
		# A lock whose owner died is reclaimed.
		owner="$(cat "$lock/pid" 2>/dev/null || true)"
		if [ -n "$owner" ] && ! kill -0 "$owner" 2>/dev/null; then
			rm -rf "$lock" "$tmp"
			continue
		fi
		if [ "$waited" -ge "$lock_wait" ]; then
			echo "dev-bootstrap: timed out waiting for another build" >&2
			return 1
		fi
		sleep 1
		waited=$((waited + 1))
	done
	echo "$$" >"$lock/pid"
	trap 'rm -rf "$lock" "$tmp"' RETURN
	[ -d "$golden" ] && return 0
	rm -rf "$tmp"
	mkdir -p "$tmp"
	cp "$web/package.json" "$web/package-lock.json" "$tmp/"
	[ -f "$web/.npmrc" ] && cp "$web/.npmrc" "$tmp/"
	(cd "$tmp" && npm ci --no-audit --no-fund) >&2 || return 1
	[ -d "$tmp/node_modules" ] || return 1
	echo "$hash" >"$tmp/node_modules/$stamp_name"
	mv "$tmp/node_modules" "$golden"
}

if [ ! -d "$golden" ]; then
	if ! build_golden; then
		echo "dev-bootstrap: could not build the shared copy (offline?)" >&2
		exit 1
	fi
	how="built shared copy"
else
	how="used shared copy"
fi

# Marks the golden copy as recently used so pruning keeps it.
touch "$golden" 2>/dev/null || true

staging="$web/.node_modules.tmp.$$"
rm -rf "$staging"
case "$(uname -s)" in
Darwin) cp -c -R "$golden" "$staging" 2>/dev/null || cp -R "$golden" "$staging" ;;
*) cp --reflink=auto -R "$golden" "$staging" 2>/dev/null || cp -R "$golden" "$staging" ;;
esac
rm -rf "$web/node_modules"
mv "$staging" "$web/node_modules"
echo "dev-bootstrap: $how, web/node_modules ready ($hash)"
