#!/bin/bash
# Exercises readme-sync.sh in a throwaway git repo; no model needed.
set -uo pipefail

SRC="$(cd "$(dirname "$0")" && pwd)/readme-sync.sh"
T="$(mktemp -d)"
trap 'rm -rf "$T"' EXIT
cd "$T" || exit 1
git init -q . && git config user.email t@example.com && git config user.name t
mkdir -p scripts/i18n docs/i18n
cp "$SRC" scripts/i18n/readme-sync.sh
S=scripts/i18n/readme-sync.sh

cat >README.md <<'R'
# Title

<!-- i18n-nav:start -->
English | [es](docs/i18n/README.es.md)
<!-- i18n-nav:end -->

[![CI](https://example.com/ci.svg)](https://example.com/ci)

## Install

```sh
curl -fsSL https://example.com/install | sh
```

See [docs](docs/a.md).
R
cat >docs/i18n/README.es.md <<'R'
# Titulo

[![CI](https://example.com/ci.svg)](https://example.com/ci)

## Instalar

```sh
curl -fsSL https://example.com/install | sh
```

Ver [documentos](../../docs/a.md).
R

fail=0
ok() { if [ "$1" -eq 0 ]; then echo "PASS $2"; else echo "FAIL $2"; fail=1; fi; }

h1="$($S hash)"
sed 's/English | \[es\].*/English | [es](x) | [de](y)/' README.md >R2 && cp README.md README.orig && cp R2 README.md
h2="$($S hash)"
[ "$h1" = "$h2" ]; ok $? "hash ignores nav block"
cp README.orig README.md

$S check | grep -q 'MISSING stamp es'; ok $? "check reports missing stamp"
$S stamp es >/dev/null; ok $? "stamp"
head -n 1 docs/i18n/README.es.md | grep -q "blob:$h1 lang:es"; ok $? "stamp line content"
git cat-file -e "$h1" 2>/dev/null; ok $? "english blob stored"
$S check | grep -q 'fresh es'; ok $? "check fresh"
printf '\nMore text.\n' >>README.md
$S check | grep -q 'STALE es'; ok $? "check stale after edit"
$S check --strict >/dev/null; [ $? -eq 1 ]; ok $? "check --strict exits 1 when stale"
$S validate es >/dev/null 2>&1; ok $? "validate passes (prose-only change)"
sed 's#install | sh#install | bash#' docs/i18n/README.es.md >T && cp T docs/i18n/README.es.md
out="$($S validate es 2>&1)"; rc=$?
[ "$rc" -eq 1 ] && echo "$out" | grep -q 'MISMATCH fences'; ok $? "validate fails on differing code block"
sed 's#install | bash#install | sh#; s#example.com/ci)#example.com/other)#' docs/i18n/README.es.md >T && cp T docs/i18n/README.es.md
out="$($S validate es 2>&1)"; rc=$?
[ "$rc" -eq 1 ] && echo "$out" | grep -q 'MISMATCH urls'; ok $? "validate fails on differing link URL"

exit "$fail"
