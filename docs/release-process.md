---
title: "Release process"
description: How a merge becomes a release, which channels publish, what is skipped when unchanged, and how to dry run, verify and repair a release.
"og:url": "https://docs.levelrail.com/release-process"
---

How `release-please.yml` and `release.yml` turn merges on `main` into a release. This page is for contributors to Levelrail itself; the checks that run on a pull request are in [CI](/ci).

## Versioning and releases

Versions are automatic. [release-please](https://github.com/googleapis/release-please) reads the conventional commit messages on `main`, picks the next version, updates `CHANGELOG.md`, and keeps one open pull request titled `chore(main): release <version>`. Merging that PR tags the release, creates the GitHub Release, and dispatches `release.yml`, which builds every binary and publishes to Homebrew and npm.

Releases are paced. A `feat` bumps the patch, not the minor, so versions go deep instead of wide; a minor jump needs the `allow-minor-release` label on the release PR. The `Release pacing` check also fails until `RELEASE_COOLDOWN_HOURS` (repository variable, default 6) have passed since the last release, so two releases cannot land in the same hour. An hourly workflow re-runs the failed check, so an armed auto-merge goes through on its own once the wait ends. The `skip-release-cooldown` label overrides the wait for an urgent fix. `scripts/test-release-pacing.sh` is the self test.

The one manual step is merging that PR. By default release-please uses the built-in `GITHUB_TOKEN`, and GitHub does not run CI on pull requests opened that way, so the required `CI required` check never reports and the PR stays blocked until someone reopens it or merges around the gate.

To make releases hands-off, give release-please a real token. A fine-grained personal access token limited to this repository, with Contents and Pull requests set to read and write, is enough:

```bash
gh secret set RELEASE_PLEASE_TOKEN --repo glincker/levelrail    # paste the token at the prompt
gh variable set AUTO_RELEASE --body true --repo glincker/levelrail
```

With the secret, the release PR runs CI like any other PR, and the tag push starts `release.yml` by itself. With the variable as well, release-please also arms auto-merge on the PR, so a change on `main` ships as a new beta once CI passes. Every merge to `main` updates the same PR, so this releases on every merge. Leave `AUTO_RELEASE` unset to keep merging release PRs by hand, with the token still doing its job of running CI on them.

## Release gate

`release.yml` has a `release-gate` job between the binary build and everything that publishes (the release assets and both container images wait on it). It runs `scripts/release-gate.sh` with the candidate `levelrail-linux-amd64`, the previous published release's binary (checksum verified against its `checksums.txt`) and the seeded databases from `scripts/upgrade-fixtures/generate.sh`. It fails the release if the schema version goes backwards, if the previous release cannot be replaced by the candidate on the same data (checked through the control plane's own `/readyz`, no Docker), or if the candidate does not refuse a newer database with exit 78. A release with no earlier release to compare against only warns. The `release` job also refuses to publish when `release-manifest.json`, the file the Updates page reads the schema from, is missing from the signed `checksums.txt`. `scripts/test-release-gate.sh` builds two binaries and checks the gate passes the good path and refuses the bad ones; it is not part of PR CI.

## Release dry run and asset check

`release-dryrun.yml` runs `scripts/release/web-build.sh`, the same script `release.yml` uses for the frontend, on every pull request into `main` and on manual dispatch, so a heap or `tsc` failure shows up on the release PR instead of on the tag. `RELEASE_NODE_HEAP_MB` (default 8192) sets the Node heap cap.

`release-verify.yml` runs after the `Release` workflow completes and when a release is published. It runs `scripts/release/verify-assets.sh <tag>`, which fails unless `checksums.txt`, `checksums.txt.sigstore.json`, the control plane, CLI and agent binaries and the agent deb and rpm packages are all attached. On failure it opens (or updates) one issue titled `Release <tag> incomplete` with the run link and the redispatch command, and closes it once the check passes. It never changes release state. `scripts/test-release-verify.sh` tests the checker with a fake `gh`.

`release.yml` also takes a `dry_run` input. `gh workflow run release.yml --ref <branch> -f tag=<existing tag> -f dry_run=true` builds the frontend, every binary and package, runs the release gate and builds both images without pushing them, then stops: no release asset, signature, image tag, package or note is published. The run is titled `Dry run <tag>`, and `release-verify.yml`, `package-managers.yml` and `release-npm-verify.yml` ignore runs with that title.

### Publishing only what changed

The `changes` job in `release.yml` runs `scripts/release/changed-components.sh` for each optional channel and writes a table to the job summary. A component counts as changed when any file its binary is built from (every Go source and embedded file `go list -deps` reports for linux, darwin and windows, plus `go.mod` and `go.sum`) or its packaging files differ from the baseline.

| Component | Baseline | Skipped when unchanged |
| --- | --- | --- |
| `mcp` | The `levelrail-mcp` version npm serves, so a failed earlier publish is retried | MCP server builds, npm publish and the MCP Registry entry |
| `terraform` | The previous `v*` tag | Terraform mirror sync |
| `cli`, `agent` | The previous `v*` tag | Nothing yet, report only |

The GitHub Release assets, the images and the CLI channels (npm, Homebrew, winget, Scoop, AUR and Nix) still ship on every release: the CLI's update hint compares against the newest GitHub Release, so skipping its package managers would leave those users with a hint they cannot act on. Detection fails open: a missing baseline, a lookup error or a failed `changes` job publishes. The `publish_all` dispatch input publishes every channel regardless, for a repair or backfill. `scripts/test-changed-components.sh` is the self test and runs in `release-dryrun.yml`.

`scripts/release/ship.sh <version>` reports release PR checks and prints the squash message. With `--after-merge` it waits (bounded by `SHIP_WAIT_SECS`, default 1800) for the tag and the Release run, then verifies the assets. It only reports; merging and dispatching stay manual.

## Publishing the CLI to Homebrew and npm

The release workflow builds every binary, attaches them to the GitHub Release, and then runs two independent jobs. A failure in one never blocks the other or the release itself.

### Homebrew

`publish-homebrew` renders `Formula/levelrail-cli.rb` from the release's `checksums.txt` and pushes it to `glincker/homebrew-tap` over SSH. It uses a deploy key that has write access to that one repo (the `HOMEBREW_TAP_DEPLOY_KEY` secret), and the step skips when the secret is missing. Users install with `brew install glincker/tap/levelrail-cli`. A personal tap has no popularity requirement; only `homebrew/core` does.

One-time setup:

```bash
ssh-keygen -t ed25519 -N "" -C levelrail-release-tap -f ./tapkey
gh repo deploy-key add ./tapkey.pub --repo glincker/homebrew-tap --title "levelrail release" --allow-write
gh secret set HOMEBREW_TAP_DEPLOY_KEY --repo glincker/levelrail < ./tapkey
rm ./tapkey ./tapkey.pub
```

The formula template is `deploy/packaging/homebrew/levelrail-cli.rb.tmpl`, and `scripts/test-packaging.sh` checks the rendered result. The binary is a raw download without the execute bit, so the template sets it with `chmod`; without that, `brew install` fails with `EACCES`.

### npm

`publish-npm` publishes `levelrail-cli` plus six platform packages (`levelrail-cli-<os>-<arch>`), then two forwarding packages that claim the brand names at the same version: `levelrail` (bin `levelrail`) and `@levelrail/cli` (bin `levelrail-scoped`, so no global bin collision). Both depend on exactly `levelrail-cli` and install the client only. They publish after `levelrail-cli` using npm trusted publishing, so no npm token is involved. Stable releases take the `latest` dist-tag and prereleases use `beta`. A version already on the registry is skipped, so re-running a partly failed release finishes the rest. A forwarder that fails to publish (for example before its first manual publish) logs a warning and does not fail the release. The job only runs when the `NPM_PUBLISH_ENABLED` repo variable is `true`.

`release-npm-verify.yml` then installs `levelrail-cli` from the registry in a clean directory on Linux (amd64 and arm64), macOS and Windows runners, checks that `levelrail-cli version` prints the release version, installs `levelrail` and `@levelrail/cli` and checks `npx --no-install levelrail version` and `levelrail-scoped version`, and runs `npm audit signatures`. It runs after the Release run completes, and only when that run's `Publish CLI to npm` job succeeded, so registry lag never holds the release open. Each lookup and install retries `NPM_VERIFY_RETRIES` times (repository variable, default 15) every `NPM_VERIFY_RETRY_SECONDS` (default 30). Dispatch it with a `tag` input to re-check a published version.

One-time setup. npm configures a trusted publisher on a package that already exists, so each package is published once by hand first. The script does the whole thing under your own npm login:

```bash
scripts/npm-bootstrap.sh --dry-run    # builds the packages and shows every step, changes nothing
scripts/npm-bootstrap.sh              # publishes, registers the trusted publisher, then enables the job
```

It downloads the newest release's binaries, builds the nine packages, publishes the platform packages first, then the launcher, then the two forwarders, runs `npm trust github` on each package for `glincker/levelrail`, workflow `release.yml`, environment `npm-publish`, and only then sets `NPM_PUBLISH_ENABLED`. It skips anything already done, so it is safe to re-run after a failure. Publishing and registering both ask for your npm two-factor code.

After that, every release publishes with no token. If a publish fails, use "Re-run failed jobs" on the workflow run; versions already on the registry are skipped.

An `NPM_TOKEN` secret, including one set at the organization level, is not used and can stay as it is for other repositories.

## Extra distribution channels (off by default)

Three optional channels run after the release job. None of them runs until its repository variable is `true`, and a missing secret skips the step instead of failing the release. Turn one on under Settings, Secrets and variables, Actions, Variables. `scripts/release/gen-manifests-test.sh` dry-runs the generators with fake checksums.

### Docker Hub mirror

Variable `DOCKERHUB_PUBLISH_ENABLED=true` makes both `merge-image-*` jobs copy the finished multi-arch index to `docker.io/<namespace>/levelrail` and `levelrail-agent` with the same tags, then cosign-sign the same digest. Secrets: `DOCKERHUB_USERNAME` and `DOCKERHUB_TOKEN` (an access token with read and write scope). Optional variable `DOCKERHUB_NAMESPACE` (default `levelrail`, a Docker Hub account dedicated to the project; the GHCR images stay under `ghcr.io/glincker`).

A Docker Hub login or mirror failure is a warning on the merge job, not a failure: the GHCR image, its signature and the release notes still publish. `dockerhub-mirror.yml` backfills the control plane image afterwards.

The Docker Hub overview texts live in `scripts/release/dockerhub/` (not under `docs/`, which the docs site publishes). Every release step that calls a `scripts/release` script skips with a notice when the script is missing on the dispatched ref; the tag step falls back to inline logic.

One-time owner steps: create the `levelrail` Docker Hub account (its profile names GLINR Studios as the company) and the two public repositories (or let the first push create them), create the access token, set the two secrets, then set the variable. The GHCR images must be public so the copy can read them.

### winget and Scoop

Variable `WINDOWS_PACKAGES_ENABLED=true` runs `publish-windows-packages` for stable tags. `scripts/release/gen-windows-manifests.sh` renders a portable-installer winget manifest set and `levelrail-cli.json` for Scoop from the two Windows `.exe` assets. The job attaches them to the release as `winget-*.yaml` and `levelrail-cli.scoop.json`, so no credential is needed for that part.

Optional secrets: `WINGET_TOKEN` (a classic personal access token with `public_repo`) lets the pinned winget-releaser action open the winget-pkgs PR; `SCOOP_BUCKET_DEPLOY_KEY` (a write deploy key on `glincker/scoop-bucket`) pushes `bucket/levelrail-cli.json`. Each step skips when its secret is missing.

One-time owner steps: submit the first `Glincker.LevelrailCli` manifest to winget-pkgs by hand (the action only updates an existing package) using the attached files, create the `glincker/scoop-bucket` repo, add the deploy key, set the secrets, then set the variable. Users run `winget install Glincker.LevelrailCli` or `scoop bucket add glincker https://github.com/glincker/scoop-bucket` then `scoop install levelrail-cli`.

### AUR and Nix

Variable `AUR_NIX_ENABLED=true` runs `publish-aur-nix`. `scripts/release/gen-aur-nix.sh` renders the `levelrail-cli-bin` PKGBUILD and a `flake.nix` that fetches the release binaries, and the job attaches them as `levelrail-cli-bin.PKGBUILD` and `levelrail-cli.flake.nix`. It needs no secrets and publishes nothing by itself.

One-time owner steps: register the `levelrail-cli-bin` package on the AUR with your own SSH key and push the attached PKGBUILD (run `makepkg --printsrcinfo > .SRCINFO` first), and copy the flake into a `glincker/nix` repo or the main repo if you want `nix run github:glincker/nix`. Automating the AUR push needs an AUR SSH key secret and is left out on purpose.
