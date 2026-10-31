#!/usr/bin/env bash
# Builds a throwaway repo with two remote branches that each add a migration and
# checks the allocator and the remote conflict warning. Usage: scripts/test-next-migration.sh
set -euo pipefail

src="$(cd "$(dirname "$0")" && pwd)"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT
fails=0
check() { # label, want, got
	if [ "$2" = "$3" ]; then echo "ok   $1"; else echo "FAIL $1: want '$2' got '$3'"; fails=1; fi
}

git init -q "$tmp/remote"
cd "$tmp/remote"
git config user.email t@example.invalid
git config user.name t
mkdir -p internal/store/migrations
echo "-- base" >internal/store/migrations/0001_base.sql
git add . && git commit -qm base
git branch -M main
git checkout -qb feat/a
echo "-- a" >internal/store/migrations/0002_alpha.sql
git add . && git commit -qm a
git checkout -q main && git checkout -qb feat/b
echo "-- b" >internal/store/migrations/0003_beta.sql
git add . && git commit -qm b
git checkout -q main && git checkout -qb feat/c
echo "-- c" >internal/store/migrations/0002_gamma.sql
git add . && git commit -qm c
git checkout -q main

git clone -q "$tmp/remote" "$tmp/work"
cd "$tmp/work"
git config user.email t@example.invalid
git config user.name t

check "next skips numbers on other remote branches" 0004 "$("$src/next-migration.sh")"
echo "-- local" >internal/store/migrations/0004_local.sql
check "working tree counts too" 0005 "$("$src/next-migration.sh")"
check "step env" 0014 "$(MIGRATION_STEP=10 "$src/next-migration.sh")"

echo "-- mine" >internal/store/migrations/0002_mine.sql
out="$("$src/check-migration-versions.sh" --against-remotes 2>&1 || true)"
case "$out" in *"0002_alpha.sql"*"0002_gamma.sql"*) echo "ok   conflict warning names both files" ;; *) echo "FAIL conflict warning: $out"; fails=1 ;; esac

rc=0
MIGRATION_REMOTE_STRICT=1 "$src/check-migration-versions.sh" --against-remotes >/dev/null 2>&1 || rc=$?
check "strict mode fails on a conflict" 1 "$rc"

exit "$fails"
