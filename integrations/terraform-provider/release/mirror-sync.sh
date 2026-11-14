#!/usr/bin/env bash
# Copies this provider directory into a clone of the mirror repo and tags it.
# Dry run by default: nothing changes without --yes. Never pushes anywhere but the mirror.
set -euo pipefail

ALLOWED_SSH="git@github.com:glincker/terraform-provider-levelrail.git"
ALLOWED_HTTPS="https://github.com/glincker/terraform-provider-levelrail.git"

usage() {
  echo "usage: mirror-sync.sh --clone <path-to-mirror-clone> --version <x.y.z> [--yes]" >&2
  exit 2
}

clone=""
version=""
yes=0
while [ $# -gt 0 ]; do
  case "$1" in
    --clone) clone="${2:-}"; shift 2 ;;
    --version) version="${2:-}"; shift 2 ;;
    --yes) yes=1; shift ;;
    *) usage ;;
  esac
done
[ -n "$clone" ] && [ -n "$version" ] || usage
case "$version" in
  [0-9]*.[0-9]*.[0-9]*) ;;
  *) echo "version must look like 1.2.3" >&2; exit 2 ;;
esac

src="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
[ -d "$clone/.git" ] || { echo "not a git clone: $clone" >&2; exit 1; }

remote="$(git -C "$clone" remote get-url origin)"
if [ "$remote" != "$ALLOWED_SSH" ] && [ "$remote" != "$ALLOWED_HTTPS" ]; then
  echo "refusing: origin is $remote, expected the mirror repo only" >&2
  exit 1
fi

if [ -n "$(git -C "$clone" status --porcelain)" ]; then
  echo "refusing: the mirror clone has uncommitted changes" >&2
  exit 1
fi

echo "source:  $src"
echo "mirror:  $clone ($remote)"
echo "would: replace the mirror working tree with the provider files (no history, no other directories)"
echo "would: commit 'chore: sync v$version' and tag v$version"
echo "would: git push origin HEAD v$version"

if [ "$yes" -ne 1 ]; then
  echo "dry run. Re-run with --yes to do it."
  exit 0
fi

# Keep .git, replace everything else, so the mirror never carries monorepo content.
find "$clone" -mindepth 1 -maxdepth 1 ! -name .git -exec rm -rf {} +
tar -C "$src" --exclude=release --exclude=e2e -cf - . | tar -C "$clone" -xf -
mkdir -p "$clone/.github/workflows"
cp "$src/release/workflow.yml.example" "$clone/.github/workflows/release.yml"

git -C "$clone" add -A
git -C "$clone" commit -m "chore: sync v$version"
git -C "$clone" tag "v$version"
git -C "$clone" push origin HEAD "v$version"
