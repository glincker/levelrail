# levelrail-cli

[![npm](https://img.shields.io/npm/v/levelrail-cli?label=npm)](https://www.npmjs.com/package/levelrail-cli)
[![npm downloads](https://img.shields.io/npm/dm/levelrail-cli)](https://www.npmjs.com/package/levelrail-cli)
[![License](https://img.shields.io/npm/l/levelrail-cli)](https://github.com/glincker/levelrail/blob/main/LICENSE)
[![OpenSSF Scorecard](https://api.scorecard.dev/projects/github.com/glincker/levelrail/badge)](https://scorecard.dev/viewer/?uri=github.com/glincker/levelrail)
[![GitHub stars](https://img.shields.io/github/stars/glincker/levelrail?style=flat&logo=github)](https://github.com/glincker/levelrail)

The Levelrail self-hosted deployment platform CLI. It is the command line client for [Levelrail](https://levelrail.com), a self-hosted PaaS: deploy, roll back and inspect apps on your own servers from your terminal.

This package installs the CLI client only. It never installs or starts the Levelrail server; to run the platform itself, follow the [install guide](https://docs.levelrail.com/installing).

## Quickstart

```
npx levelrail-cli help
npm install --global levelrail-cli
yarn dlx levelrail-cli help
pnpm dlx levelrail-cli help
```

Point it at your instance and log in:

```
export APP_API_URL=https://your-dashboard-domain
levelrail-cli auth login --device
```

## Commands

`levelrail-cli help` lists the everyday verbs and `levelrail-cli help advanced` lists the rest:

- `login`, `status`, `open`: sign in, check the instance, print an app's URLs
- `up`, `deploy`, `rollback`: ship a directory or an image, go back one deploy
- `logs`, `env`, `domains`: inspect and configure an app
- `db`, `backup`, `restore`: databases and their backups
- `apps`: everything else about apps

Full reference: [CLI reference](https://docs.levelrail.com/cli-reference).

## Verify what you install

The package installs a prebuilt binary for your platform (Linux, macOS or Windows, x64 or arm64) through optional dependencies. It has no install scripts. Published with npm provenance from GitHub Actions, so you can check where it was built:

```
npm audit signatures
```

Release binaries are also checksummed and signed; see [Installing](https://docs.levelrail.com/installing).

## Links

- Website: [levelrail.com](https://levelrail.com)
- Docs: [docs.levelrail.com](https://docs.levelrail.com)
- Source: [github.com/glincker/levelrail](https://github.com/glincker/levelrail)
- Issues: [github.com/glincker/levelrail/issues](https://github.com/glincker/levelrail/issues)

Apache 2.0.
