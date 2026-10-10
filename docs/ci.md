---
description: How the GitHub Actions checks on a pull request are chosen, what they cost, and which ones branch protection relies on.
---

# CI

How the GitHub Actions checks on a pull request are chosen and which ones gate a merge. This page is for contributors to Levelrail itself.

<InlineToc default-open />

## The short version

A PR runs only the work its diff can affect:

- **Docs only** (`*.md`, `adr/`): change detection, the aggregator, the
  secret scan and PR hygiene. A change under `docs/` also rebuilds the web
  bundle, since `docs/` feeds the dashboard's `/help` pages.
- **Web only** (`web/`): the web job, with vitest limited to tests whose
  module graph reaches a changed file. No Go job runs.
- **Go**: build, vet and tests for the affected packages only (the packages
  you changed plus every package that imports them, including through test
  imports), lint for the packages you changed, reporting only issues on
  lines you touched.
- **Workflow files only**: actionlint on the changed workflow files, inside
  the Lint job.

A `main` push runs the same impact-based jobs as a PR (`Build, vet`, `Lint`,
every `Test (*)` job, `Coverage gate`, `Web (tsc, eslint)`, `install.sh`),
scoped against `github.event.before` and falling back to `--full` when that
SHA is missing or unresolvable. The merge queue is gone, so this is the only
post-merge verification of the merged tree. Docs-only pushes never trigger
`ci.yml` at all (`paths-ignore` on the `push` trigger: `**/*.md`, `adr/**`,
`LICENSE`), and an area with no changed files skips as on a PR.
`nightly.yml` runs the full `-race`, no `-short` sweep once a day regardless:
the safety net for anything the scoping under-selects.

## How a change is scoped

Two scripts decide, and both can be run locally against any diff.

`scripts/ci-changes.sh <base> [head]` (the **Detect changed areas** job, no
Go toolchain) sorts changed files into areas:

| Changed path | Effect |
| --- | --- |
| `.github/workflows/ci.yml`, `scripts/ci-*.sh`, `scripts/affected-go-packages.sh`, `scripts/go-test-groups.sh`, the coverage, lane and migration check scripts | Full run of everything: the pipeline itself changed |
| `go.mod`, `go.sum`, `internal/store/migrations/*.sql` | Every Go package, full lint (same rule as the pre-push hook) |
| `.golangci.yml` | Full lint, plus the kit lane |
| `kit/*.go`, `kit/go.mod`, `kit/go.sum` | The kit lane, plus the root packages that import the changed kit package (`kit/go.mod` and `kit/go.sum` run every Go package) |
| `*.go` | That package, plus its dependents; lint for that package |
| Non-Go file inside a Go package (embedded schema, SQL, `testdata/`) | That package |
| File outside any package (`docs/`, root files, `proto/`) | Packages whose Go files name it in a string literal, for example `internal/api` for `docs/api-reference.md` |
| `web/` | Web job; config, lockfile or test setup changes run the full vitest suite |
| `docs/*.md` | Web bundle build and the help page tests |
| `install.sh`, `scripts/test-install-sh.sh`, `packaging/` | install.sh end to end on Ubuntu and Debian |
| Other `.github/workflows/*` | actionlint on the changed files |
| `.github/flaky-tests.txt` | The packages named in the changed lines, plus the quarantine lane |
| Anything else (`adr/`, `LICENSE`, other `.github/` files) | Nothing beyond the always-on checks |

`scripts/ci-go-plan.sh <base> [head]` (inside **Plan Go work**) turns the Go
side into packages and test lanes through `scripts/affected-go-packages.sh`,
the same dependency-graph walk the pre-push hook uses. A change only a
package's own tests can see (`_test.go`, `testdata/`, a doc a test reads)
selects that package alone, not its dependents. Lanes:

- `internal/api` in four shards by test name whenever it is affected. It is
  the slowest package by far, and most backend changes reach it.
- `test/e2e` always gets its own `e2e` lane when it is affected, so the
  heaviest package sets the floor alone. On a full live run
  (see [Live suites](#live-suites)) the template fleet test gets its own
  `e2e-fleet` lane, `e2e` runs everything else in that package, and
  `test/e2e/reconcile` gets an `e2e-reconcile` lane.
- Up to `CI_SMALL_LANE_MAX` (default 8) other affected packages share one
  `rest` lane. Above that, Docker-backed packages get their own `docker`
  lane with bounded `-p` and the rest share `rest`.
- No lane at all when nothing Go-related is affected.

The lanes and the checks that gate them:

| Lane | Packages | Present when | Required check name |
| --- | --- | --- | --- |
| `api-1` to `api-4` | `internal/api`, sharded by test name | `internal/api` affected | `Test (api-N)`, aggregated as `Test (internal/api)` |
| `e2e-fleet` | `test/e2e`, template fleet test only | full live run | `Test (e2e-fleet)` |
| `e2e` | `test/e2e` (minus the fleet test on a full live run) | `test/e2e` affected | `Test (e2e)` |
| `e2e-reconcile` | `test/e2e/reconcile` | full live run, package affected | `Test (e2e-reconcile)` |
| `docker` | other Docker-backed packages, `-p 2` | many packages affected | `Test (docker)` |
| `rest` | everything else (and the Docker ones on a small diff) | any other package affected | `Test (rest)` |

Every planned lane is listed in `rest_checks` or `api_checks`, which the
`Test (everything else)` and `Test (internal/api)` aggregators wait on, and
`scripts/test-ci-go-plan.sh` asserts that for each case, so a lane cannot be
added without a check that fails when it fails or goes missing. Planning runs
in its own **Plan Go work** job, so the test lanes start as soon as the plan
exists instead of waiting behind `go build` and `go vet`.

`test/e2e` is split from `test/e2e/reconcile` along one dependency: only the
former constructs a live `api.Router`. Since `internal/api` is the package
most PRs touch, this keeps a one-file fix there from selecting the whole
end-to-end suite through the test-import walk. The reconciler and Docker-only
tests in `test/e2e/reconcile` run only when a PR's diff reaches something they
import.

## Live suites

The heaviest tests pull real upstream images and run real container
lifecycles: the template fleet (`TestTemplateFleet_Live_DeploysAndTearsDownCleanly`,
about 1,000 s when run one template at a time), the catalog batch live tests
and the break-glass tests. They are the only tests the PR gate trims. Every
other `test/e2e` and `test/e2e/reconcile` test, and every unit test, runs on
every PR the same as before.

`scripts/ci-go-plan.sh` decides `live=full` or `live=smoke` for the PR, in one
place, and prints the reason in the **Go plan** job summary:

| `live` | When | What runs |
| --- | --- | --- |
| `full` | The diff touches `internal/catalog`, `internal/compose`, `internal/registrycatalog`, `internal/reconcile`, `internal/docker`, `internal/dockertest`, `internal/agent`, `internal/build`, `internal/pipeline`, `internal/ingress`, `test/e2e`, `ci.yml`, `nightly.yml`, `.github/actions`, or the planner, lane runner or group scripts | The whole fleet (28 templates), every catalog batch, both break-glass tests |
| `full` | The PR has the `ci:live` label | Same |
| `full` | A full run (`go.mod`, migrations, pipeline scripts, push with no base) | Same |
| `smoke` | Anything else | Fleet limited to `uptime-kuma`, `mailpit` and `gitea` (one single-service, one tiny, one db-backed multi-service template); the catalog batch and break-glass tests skip |

A skipped heavy test is never silent. It skips with the message
`skipped by design: LEVELRAIL_LIVE_SUITE=smoke`, and the lane's job summary
lists every test skipped that way under "Skipped by design". The mechanism is
`LEVELRAIL_LIVE_SUITE` in `test/e2e/testenv`; unset (local runs and
`nightly.yml`) means everything runs.

To force the full set on a PR: add the `ci:live` label (create it once with
`gh label create ci:live`), then re-run all jobs or push. The plan job reads
the labels live, because a re-run replays the original event payload.

`nightly.yml` always keeps the docker lane in its matrix
(`NIGHTLY_LIVE_ALWAYS`), so the full live set runs every night even on a day
with no commits, which is when an upstream image change would otherwise go
unnoticed.

Run the full set on a developer machine with one command:

```sh
scripts/live-tests.sh                       # all of test/e2e and test/e2e/reconcile
scripts/live-tests.sh -run TestBreakGlass   # one suite
```

The fleet test deploys `LEVELRAIL_FLEET_PARALLEL` templates at once (default
3, `1` for strictly sequential). Each template gets its own app id, network
and volumes, and teardown asserts nothing leaked per template.

The coverage gate follows the plan: a full run checks the 70% aggregate
for `internal/`, a scoped run checks the changed-line gate only, at a
lower 50% bar (a partial profile makes the aggregate meaningless, same as the
pre-push hook).

## The kit module

`kit/` is a nested Go module (`github.com/GLINCKER/levelrail/kit`), so
`go test ./...`, `go vet ./...` and `golangci-lint run` from the repo root
never reach it. It has its own lane instead:

- **Kit (vet, lint, test)** runs only when a diff touches `kit/` (not `kit/*.md`),
  `.golangci.yml`, or the pipeline itself. It runs `go -C kit vet`, golangci-lint
  with the root `.golangci.yml` (found by walking up from `kit/`),
  `go -C kit test -short` and `scripts/check-brand-strings.sh`.
- Coverage: a 70% floor for all of `kit/` on every run of the lane, and the 50%
  changed-line bar from the main gate, both through the same coverage scripts
  with the `kit/` prefix.
- Dependents: `scripts/affected-go-packages.sh` maps a changed kit package to its
  import path and walks the root import graph, so root packages that import it are
  tested in the normal lanes. Test-only kit changes select no root package.
- The lane is a `needs` of `CI required`, so a skipped lane (no kit change) passes
  and a failed or cancelled one blocks.
- `nightly.yml` runs `go -C kit test -race -shuffle=on` without `-short`.
- Releases are independent: release-please tags the module `kit/vX.Y.Z`, which does
  not match `release.yml`'s `v*` tag filter, so it never starts a product release.
  The kit package carries `release-as: 0.1.0` until its first release ships; remove
  it afterwards.

## Required checks

**CI required** is the aggregator and the one check meant to gate a merge. It always runs and needs `changes`, `plan`, `build-vet`, `lint`, `kit`, the Go test jobs, `coverage-gate`, `web-check` and `install-sh`. It fails if change detection did not succeed or any job it needs failed or was cancelled, and a job that was skipped because its area had no work counts as passing. Because branch protection can name just this one context, the job set can be split or renamed without touching the ruleset. Branch protection itself is configured in GitHub, not in this repository.

Seven jobs also never skip because of an upstream failure: `Plan Go work`, `Build, vet`, `Lint`, `Test (internal/api)`, `Test (everything else)`, `Coverage gate` and `Web (tsc, eslint)`. Each runs, and fails, whenever an upstream job failed or was cancelled, and is skipped only when its area has no work for the diff. GitHub reports a job skipped by its own `if` as success for a required check, so auto-merge does not wait on it.

## Other workflows on a PR

| Workflow | On a PR | Why |
| --- | --- | --- |
| `secret-scan.yml` (gitleaks) | Every PR, commits in the change only | A token leaked in a markdown file is still a leak. Uses the pinned release binary, no Go build |
| `pr-hygiene.yml` | Every PR | Labels, size, description and commit rules in one job |
| `codeql.yml` | No | Not a required check. Runs on pushes to `main` that touch Go or web sources, weekly, and on manual dispatch |
| `dependabot-auto-merge.yml` | Dependabot PRs only (listed as skipped elsewhere) | |
| SonarCloud | Every PR | A GitHub App, not an Actions job: it runs on the vendor's infrastructure, uses no Actions minutes, and is not a merge blocker |

## Scheduled, not PR-triggered

- `branch-cleanup.yml`: deletes a merged PR's head branch immediately, plus a
  weekly sweep (Monday) for anything left over from before branch deletion
  on merge was enabled.
- `backup-tags.yml`: moves `backup/daily` to `main`'s tip every day, and
  `backup/weekly` on Mondays, as a known-name emergency rollback target.
  Tags, not branches, so they never show up in the list the cleanup above
  is shrinking. Every commit on `main` is already a valid, permanent
  rollback point on its own (no force-push, no deletion); these tags exist
  only so finding one doesn't mean hunting for a SHA by hand first.

## Versioning and releases

Versions are automatic. [release-please](https://github.com/googleapis/release-please) reads the conventional commit messages on `main`, picks the next version (a `v0.2.0-beta.N` pre-release while no stable release exists), updates `CHANGELOG.md`, and keeps one open pull request titled `chore(main): release <version>`. Merging that PR tags the release, creates the GitHub Release, and dispatches `release.yml`, which builds every binary and publishes to Homebrew and npm.

The one manual step is merging that PR. By default release-please uses the built-in `GITHUB_TOKEN`, and GitHub does not run CI on pull requests opened that way, so the required `CI required` check never reports and the PR stays blocked until someone reopens it or merges around the gate.

To make releases hands-off, give release-please a real token. A fine-grained personal access token limited to this repository, with Contents and Pull requests set to read and write, is enough:

```bash
gh secret set RELEASE_PLEASE_TOKEN --repo glincker/levelrail    # paste the token at the prompt
gh variable set AUTO_RELEASE --body true --repo glincker/levelrail
```

With the secret, the release PR runs CI like any other PR, and the tag push starts `release.yml` by itself. With the variable as well, release-please also arms auto-merge on the PR, so a change on `main` ships as a new beta once CI passes. Every merge to `main` updates the same PR, so this releases on every merge. Leave `AUTO_RELEASE` unset to keep merging release PRs by hand, with the token still doing its job of running CI on them.

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

The formula template is `packaging/homebrew/levelrail-cli.rb.tmpl`, and `scripts/test-packaging.sh` checks the rendered result. The binary is a raw download without the execute bit, so the template sets it with `chmod`; without that, `brew install` fails with `EACCES`.

### npm

`publish-npm` publishes `levelrail-cli` plus six platform packages (`levelrail-cli-<os>-<arch>`) using npm trusted publishing, so no npm token is involved. While no stable release exists, a prerelease takes the `latest` dist-tag so `npm install levelrail-cli` gets the newest build; after the first stable release, prereleases use `beta`. A version already on the registry is skipped, so re-running a partly failed release finishes the rest. The job only runs when the `NPM_PUBLISH_ENABLED` repo variable is `true`.

One-time setup. npm configures a trusted publisher on a package that already exists, so each package is published once by hand first. The script does the whole thing under your own npm login:

```bash
scripts/npm-bootstrap.sh --dry-run    # builds the packages and shows every step, changes nothing
scripts/npm-bootstrap.sh              # publishes, registers the trusted publisher, then enables the job
```

It downloads the newest release's binaries, builds the seven packages, publishes the platform packages first, runs `npm trust github` on each package for `glincker/levelrail`, workflow `release.yml`, environment `npm-publish`, and only then sets `NPM_PUBLISH_ENABLED`. It skips anything already done, so it is safe to re-run after a failure. Publishing and registering both ask for your npm two-factor code.

After that, every release publishes with no token. If a publish fails, use "Re-run failed jobs" on the workflow run; versions already on the registry are skipped.

An `NPM_TOKEN` secret, including one set at the organization level, is not used and can stay as it is for other repositories.

## Caching

- Go: `~/.cache/go-build` and `~/go/pkg/mod`, one cache per job kind (build,
  lint, and each test lane group), keyed on the Go version and `go.sum` plus
  `tools/go.sum`. Pushes to `main` save one fresh cache per day. PR runs only
  restore, so they cannot churn the repository's 10 GB cache quota. Every
  job that needs this shares `.github/actions/setup-go-cached` rather than
  repeating setup-go plus restore/save inline (five jobs did, byte-for-byte
  identical except the cache-key prefix).
- golangci-lint: the action's own analysis cache, saved on `main` only.
- npm: `actions/setup-node`'s npm cache, keyed on `web/package-lock.json`.
- tsc: `web/node_modules/.tmp` (the `tsc -b` build info), restored on PRs and
  saved on `main` pushes. A stale build info is safe: tsc compares file hashes.

## Web jobs

`Web (tsc, eslint)` builds, checks the bundle budget, lints changed files and
runs the related vitest tests. When the whole vitest suite is selected
(config, lockfile or test-setup changes), it runs separately as
`Web tests (1/4)` to `Web tests (4/4)` with `vitest --shard`, because vitest
runs files serially (`fileParallelism: false`, to avoid jsdom contention
flakes) and one job would be the suite's whole wall time. A skipped shard job
passes `CI required` the same way a skipped area does.

## Concurrency and merge queue

A new push to a PR cancels that PR's previous CI run. Runs on `main` are
never cancelled.

There is no merge queue on `main` now, so a PR merges as soon as its own
`CI required` check passes, and the `push` run on `main` is the only
verification of the merged tree. That run does everything a PR run does,
scoped by the same impact detection, so a change that was fine alone but
breaks against a newer `main` is caught the same day, not at the nightly
sweep.

`ci.yml` still listens for `merge_group`, with the same base-commit scoping
as a PR, so re-enabling a queue needs only the branch protection setting.
If a queue returns, the `push` run would duplicate the `merge_group` run on
the same tree, so put a `github.event_name != 'push'` guard back on the heavy
jobs at that point.

`Build, vet` also refreshes the daily Go build cache on `push`
(the cache `save` flag, true only for push events), which PR runs depend on
staying warm. `codeql.yml` and `secret-scan.yml` are separate workflow
files with their own `push` triggers, unaffected by anything in `ci.yml`.

## Check what a diff would run

Both scoping scripts run locally against any pair of commits, which is the quickest way to see what a change selects:

```sh
scripts/ci-changes.sh <base-sha> <head-sha>
scripts/ci-go-plan.sh <base-sha> <head-sha>
```

## Tuning

- `CI_SMALL_LANE_MAX`: the package count at or under which non-api
  packages share one lane (default 8), read by `scripts/ci-go-plan.sh`.
- To force the full suite on a PR, change `.github/workflows/ci.yml` or one
  of the pipeline scripts, or run the nightly workflow on the branch with
  `workflow_dispatch`.
- The e2e and docker lanes' `go test -timeout` (27m, job `timeout-minutes: 32`)
  has headroom above the fleet test's cost. The fleet test now runs its
  templates in bounded parallel (`LEVELRAIL_FLEET_PARALLEL`), and the PR
  smoke subset keeps the typical `test/e2e` lane in the low minutes.
- Test databases: `internal/store`, `internal/pipeline` and `internal/api`
  tests start from a copy of one migrated template database instead of
  replaying every migration per test. New test helpers that open a store in
  a hot path should do the same.
