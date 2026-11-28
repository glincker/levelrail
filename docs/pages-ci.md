---
title: "Deploy Pages from CI"
description: "Create a Pages deploy token that can publish one site only, then deploy from GitHub Actions or any other CI with the pages-deploy action or the CLI."
---

A deploy token can publish one Pages site and nothing else. Create one per pipeline and rotate it when you like.

```bash
levelrail-cli pages token create --site docs --ttl 90d --label ci
```

The token is printed once. `--ttl` accepts `30d` or `720h`; the server caps it with its token expiry setting. List and revoke with `pages token list docs` and `pages token revoke docs <id>`.

## GitHub Actions

Store the token as the secret `LEVELRAIL_PAGES_TOKEN`.

```yaml
name: pages
on: [push, pull_request]
permissions:
  contents: read
  pull-requests: write
jobs:
  deploy:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - run: npm ci && npm run build
      - uses: GLINCKER/levelrail/actions/pages-deploy@v1.5.7
        with:
          version: v1.5.7
          site: docs
          dir: ./dist
          server: https://levelrail.example.com
          token: ${{ secrets.LEVELRAIL_PAGES_TOKEN }}
```

The action downloads the CLI from the GitHub release, checks it against `checksums.txt`, deploys, and comments the live URL on pull requests.

## Any other CI

Download, verify, run. Pin the release tag.

```bash
TAG=v1.5.7
ASSET=levelrail-cli-linux-amd64
BASE=https://github.com/glincker/levelrail/releases/download/$TAG
curl -fsSLO "$BASE/$ASSET" && curl -fsSLO "$BASE/checksums.txt"
grep " $ASSET\$" checksums.txt | sha256sum -c -
chmod +x "$ASSET"
APP_API_URL=https://levelrail.example.com APP_API_TOKEN="$LEVELRAIL_PAGES_TOKEN" \
  ./"$ASSET" pages deploy ./dist --name docs
```

Keep the token in your CI secret store and never echo it.
