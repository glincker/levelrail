# Levelrail Setup CLI

Installs `levelrail-cli` on a Linux or macOS runner and adds it to `PATH`. The release checksum is verified (and the cosign signature too, when `cosign` is on the runner).

```yaml
- uses: glincker/levelrail/.github/actions/setup-cli@main
  with:
    version: v0.2.0-beta.15   # optional, defaults to the newest release
- run: levelrail-cli apps status my-app
  env:
    APP_API_URL: ${{ secrets.LEVELRAIL_API_URL }}
    APP_API_TOKEN: ${{ secrets.LEVELRAIL_API_TOKEN }}
```

To deploy an image, use [`deploy`](../deploy/README.md), which does this install itself.
