---
title: "CI"
description: How the GitHub Actions checks on a pull request are chosen, what they cost, and which ones branch protection relies on.
"og:url": "https://docs.levelrail.com/ci"
---

How the GitHub Actions checks on a pull request are chosen and which ones gate a merge. This page is for contributors to Levelrail itself.

## The short version

A PR runs only the work its diff can affect:

- Docs only (`*.md`, `adr/`): change detection, the aggregator, the
  secret scan and PR hygiene. A change under `docs/` also rebuilds the web
  bundle, since `docs/` feeds the dashboard's `/help` pages.
- Web only (`web/`): the web job, with vitest limited to tests whose
  module graph reaches a changed file. No Go job runs.
- Go: build, vet and tests for the affected packages only (the packages
  you changed plus every package that imports them, including through test
  imports), lint for the packages you changed, reporting only issues on
  lines you touched.
- Workflow files only: actionlint on the changed workflow files, inside
  the Lint job.

A store migration that only adds tables, indexes or columns doesn't run
everything. It selects the store and the packages that mention the table.
Whole-tree scanner tests have no import edge to what they scan, so
`scripts/ci-guards.txt` lists them and any non-test Go change selects them.

A `main` push runs the same impact-based jobs as a PR (`Build, vet`, `Lint`,
every `Test (*)` job, `Coverage gate`, `Web (tsc, eslint)`, `install.sh`),
scoped against `github.event.before` and falling back to `--full` when that
SHA is missing or unresolvable. The merge queue is gone, so this is the only
post-merge verification of the merged tree. Docs-only pushes never trigger
`ci.yml` at all (`paths-ignore` on the `push` trigger: `**/*.md`, `adr/**`,
`LICENSE`), and an area with no changed files skips as on a PR.
`nightly.yml` runs the full no `-short`, shuffled sweep once a day, as the safety
net for anything the scoping under-selects. It diffs against the last
green nightly, so a lane that failed or timed out is retried until it
passes. `-race` runs every night on the packages that start goroutines or use
`sync`, `atomic` or channels (`scripts/go-test-groups.sh race`), and on the api
and docker lanes on Sundays or a manual dispatch. The `rest` packages split on
`scripts/ci-heavy-packages.txt` so no lane nears its timeout, and the docker
lane pre-pulls its upstream images with retries.

## How a change is scoped

Two scripts decide, and you can run both locally against any diff.

`scripts/ci-changes.sh <base> [head]` (the Detect changed areas job, no
Go toolchain) sorts changed files into areas:

| Changed path | Effect |
| --- | --- |
| `.github/workflows/ci.yml`, `scripts/ci-*.sh`, `scripts/affected-go-packages.sh`, `scripts/go-test-groups.sh`, the coverage, lane and migration check scripts | Full run of everything: the pipeline itself changed |
| `go.mod`, `go.sum` | Every Go package, full lint (same rule as the pre-push hook) |
| `internal/store/migrations/*.sql`, additive only (new table, index or column) | `internal/store` plus the packages whose Go code names the touched table, own tests only |
| `internal/store/migrations/*.sql` that drops, renames, updates, deletes, creates a view, or adds a trigger on an existing table | Every Go package (it can change behavior anywhere) |
| Any non-test `*.go` | Also the guard packages in `scripts/ci-guards.txt` (whole-tree scanners such as `test/execgate`, seconds each) |
| `.golangci.yml` | Full lint, plus the kit lane |
| `kit/*.go`, `kit/go.mod`, `kit/go.sum` | The kit lane, plus the root packages that import the changed kit package (`kit/go.mod` and `kit/go.sum` run every Go package) |
| `*.go` | That package, plus its dependents; lint for that package |
| Non-Go file inside a Go package (embedded schema, SQL, `testdata/`) | That package |
| File outside any package (`docs/`, root files, `proto/`) | Packages whose Go files name it in a string literal, for example `internal/api` for `docs/api-reference.md` |
| `web/` | Web job; config, lockfile or test setup changes run the full vitest suite |
| `docs/*.md` (except `docs/changelog/`, which the in-app help does not bundle) | Web bundle build and the help page tests |
| `scripts/test-ci-*.sh`, `scripts/ci-test-weights.txt`, `scripts/refresh-test-weights.sh` | The planner table tests (`Build, vet`), no Go test lane |
| `.github/actions/setup-go-cached/` | Every Go lane (live suites in smoke mode) |
| `install.sh`, `install-cli.sh`, `install-cli.ps1`, `scripts/test-install-*`, `deploy/packaging/` | installers end to end: Ubuntu and Debian containers, plus the CLI installer natively on macOS and Windows |
| Other `.github/workflows/*` | actionlint on the changed files |
| `.github/flaky-tests.txt` | The packages named in the changed lines, plus the quarantine lane |
| Anything else (`adr/`, `LICENSE`, other `.github/` files) | Nothing beyond the always-on checks |

`scripts/ci-go-plan.sh <base> [head]` (inside the Plan Go work job) turns the Go
side into packages and test lanes through `scripts/affected-go-packages.sh`,
the same dependency-graph walk the pre-push hook uses. A change only a
package's own tests can see (`_test.go`, `testdata/`, a doc a test reads)
selects that package alone, not its dependents. Lanes:

- `internal/api` in four shards by test name whenever it is affected. It is
  the slowest package by far, and most backend changes reach it. Tests are
  packed onto shards by `scripts/ci-shard-names.sh`, heaviest first onto the
  least loaded shard, using the seconds in `scripts/ci-test-weights.txt`
  (unlisted tests weigh 0.28 s). Refresh the file from recent
  `test-results-api-*` artifacts with `scripts/refresh-test-weights.sh`.
- `test/e2e` always gets its own `e2e` lane when it is affected, so the
  heaviest package sets the floor alone. On a full live run
  (see [Live suites](#live-suites)) the template fleet test gets its own
  `e2e-fleet` lane, `e2e` runs everything else in that package, and
  `test/e2e/reconcile` gets `e2e-reconcile` and `e2e-reconcile-2`: the three
  catalog batch tests (about 200 s of the package's 430 s) run in the second
  lane, every other test, including any new one, in the first.
- Up to `CI_SMALL_LANE_MAX` (default 8) other affected packages share one
  `rest` lane. Above that, Docker-backed packages get their own `docker`
  lane with bounded `-p` and the rest share `rest`.
- No lane at all when nothing Go-related is affected.

The lanes and the checks that gate them:

| Lane | Packages | Present when | Required check name |
| --- | --- | --- | --- |
| `api-1` to `api-4` | `internal/api`, sharded by test name | `internal/api` affected | `Test (api-N)`, aggregated as `Test (internal/api)` |
| `e2e-fleet` | `test/e2e`, template fleet test only | `test/e2e` affected and a fleet path changed ([below](#which-e2e-lanes-run)) | `Test (e2e-fleet)` |
| `e2e` | `test/e2e` (minus the fleet test on a full live run) | `test/e2e` affected and an e2e path changed | `Test (e2e)` |
| `e2e-reconcile` | `test/e2e/reconcile`, minus the three catalog batch tests on a full live run | package affected and a fleet path changed | `Test (e2e-reconcile)` |
| `e2e-reconcile-2` | the three catalog batch tests of `test/e2e/reconcile` | as above, full live run only (smoke mode runs one lane) | `Test (e2e-reconcile-2)` |
| `docker` | other Docker-backed packages, `-p 2` | many packages affected | `Test (docker)` |
| `rest` | everything else (and the Docker ones on a small diff) | any other package affected | `Test (rest)` |

Every planned lane is listed in `rest_checks` or `api_checks`, which the
`Test (everything else)` and `Test (internal/api)` aggregators wait on, and
`scripts/test-ci-go-plan.sh` asserts that for each case, so a lane cannot be
added without a check that fails when it fails or goes missing. Planning runs
in its own Plan Go work job, so the test lanes start as soon as the plan
exists instead of waiting behind `go build` and `go vet`.

`test/e2e` is split from `test/e2e/reconcile` along one dependency: only the
former constructs a live `api.Router`. Since `internal/api` is the package
most PRs touch, this keeps a one-file fix there from selecting the whole
end-to-end suite through the test-import walk. The reconciler and Docker-only
tests in `test/e2e/reconcile` run only when a PR's diff reaches something they
import.

## Which e2e lanes run

The three live-Docker lanes are the most expensive jobs on a PR (each about
6 to 7 minutes of runner time, often all three at once). The plan only
schedules a lane when the diff can reach what it exercises. A lane that is
not planned is not in `rest_checks`, so `Test (everything else)` does not wait
on it and `CI required` passes without it.

| Lane | Runs when the diff touches |
| --- | --- |
| `e2e-fleet`, `e2e-reconcile` | `internal/reconcile/` (a diff confined to `internal/reconcile/ingress/` does not count), `internal/agent/`, `internal/network/`, `test/e2e/` |
| `e2e` | `internal/ingress/`, `internal/reconcile/` (including `ingress/`), `internal/docker/`, `internal/deploy/`, `test/e2e/` |
| all three | The `ci:live` label, a full run, or a change to `ci.yml`, `nightly.yml`, `.github/actions/` or the planner, lane runner or group scripts (alongside a diff that reaches `test/e2e`) |

`nightly.yml` is unchanged and runs every lane and the full live set, so a
change this table skips is still verified within a day. Add `ci:live` to a PR
that changes something the table does not list but that a template deploy
depends on (for example `internal/catalog`).

`scripts/test-ci-go-plan.sh` holds the table cases: an ingress-only diff
plans `e2e` and neither `e2e-fleet` nor `e2e-reconcile`, a reconcile diff
plans all three, a docs-only diff plans none, and `ci:live` plans all.

## Live suites

The heaviest tests pull real upstream images and run real container
lifecycles: the template fleet (`TestTemplateFleet_Live_DeploysAndTearsDownCleanly`,
about 1,000 s when run one template at a time), the catalog batch live tests
and the break-glass tests. They are the only tests the PR gate trims. Every
other `test/e2e` and `test/e2e/reconcile` test, and every unit test, runs on
every PR the same as before.

`scripts/ci-go-plan.sh` decides `live=full` or `live=smoke` for the PR, in one
place, and prints the reason in the Go plan job summary:

| `live` | When | What runs |
| --- | --- | --- |
| `full` | The diff touches `internal/catalog`, `internal/compose`, `internal/registrycatalog`, `internal/reconcile`, `internal/docker`, `internal/dockertest`, `internal/agent`, `internal/network`, `internal/build`, `internal/pipeline`, `internal/ingress`, `test/e2e`, `ci.yml`, `nightly.yml`, `.github/actions`, or the planner, lane runner or group scripts | The whole fleet (28 templates) when `e2e-fleet` is planned, every catalog batch, both break-glass tests |
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

## What runs where

PRs carry a thin live smoke. The full live sweep belongs to nightly and to
the release gate. Three mechanisms decide it, all already in the pipeline:

- `go test -short` (every PR lane): a live Docker, DB or network test calls
  `dockertest.SkipIfShort(t)` (or goes through a helper that does, such as
  `liveClient` and `liveRuntime`) and is skipped. `nightly.yml` runs without
  `-short`, so it runs there, shuffled, with its images pre-pulled.
- `testenv.RequireFullLive(t)` (e2e lanes): the test skips in smoke mode, which
  is every PR whose diff does not touch the area (see `live=full` above).
  A diff that touches `internal/{docker,agent,ingress,reconcile,...}` or
  `test/e2e` still runs it, so a change to the code a heavy test covers is
  checked by that test before merge.
- The `ci:live` label forces `live=full` on any PR.

| Tier | What | Where |
| --- | --- | --- |
| PR, always | Unit tests, fake-runtime and httptest tests (authz, secret-leak, supply chain, migration state machine, envelope-secret env, compose healthcheck gate, scheduled task concurrency, loopback ingress, local-repo git clone and branch listing), the e2e smoke subset | `ci.yml` |
| PR, when the diff touches the area | Heavy e2e live tests (`RequireFullLive`): load balancer real traffic and drain, point-in-time restore over HTTP, compose healthcheck gate, catalog batches, break-glass, template fleet | `ci.yml` with `live=full` |
| Nightly and release gate only | Everything skipped by `-short`: DB restore, clone, PITR and volume live tests, docker exec TTY, agent remote build and exec TTY, scheduled task and pipeline live, ACME, the migration runner live test, secret-env live test | `nightly.yml`, and the full run when a release branch merges to `main` |
| Weekly | Template smoke across the catalog | `template-smoke.yml` |

CI time budget: a PR that touches no live-suite path should finish its Go
lanes in under 8 minutes of wall time, and the live-heavy lanes (`docker`,
`e2e*`) should stay under 25 lane-minutes combined in smoke mode. Anything
that pushes past that moves to nightly.

### Adding a live test

The default is nightly. A new test that needs Docker, a database, the network
or a real registry starts with `dockertest.SkipIfShort(t)` (unit and package
tests) or `testenv.RequireFullLive(t)` (`test/e2e`). Promote it to run on every
PR only when it guards a property that no fake can (and say which one in the
PR). Write the fake-backed twin first: a logic assertion belongs in an
httptest or fake-runtime test, and the live test then proves only what the
fake cannot, the real daemon or real wire behavior.

### Retries

Retry infrastructure errors, never assertion failures. Registry pulls retry
with backoff in two places: `scripts/prepull-test-images.sh` runs before the
`docker` and `e2e*` lanes (and nightly) so a 429 from a public registry costs a
backoff, not a re-run of the job, and `test/e2e/testenv/pull_retry.go` retries
`Create` on 429, 5xx and timeout errors inside the test. `scripts/ci-go-test.sh`
reruns a failed test twice on the `docker` and `e2e*` lanes only, where the
failure can be infrastructure; every other lane (`api-*`, `rest`) has zero
reruns, so a real failure is red the first time and a test that passes only on
retry cannot hide there.

## The kit module

`kit/` is a nested Go module (`github.com/GLINCKER/levelrail/kit`), so
`go test ./...`, `go vet ./...` and `golangci-lint run` from the repo root
never reach it. It has its own lane instead:

- The Kit (vet, lint, test) lane runs only when a diff touches `kit/` (not `kit/*.md`),
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

`CI required` is the aggregator and the one check meant to gate a merge. It always runs and needs `changes`, `plan`, `build-vet`, `lint`, `kit`, the Go test jobs, `coverage-gate`, `web-check`, `install-sh` and the macOS and Windows CLI installer jobs. It fails if change detection did not succeed or any job it needs failed or was cancelled, and a job that was skipped because its area had no work counts as passing. Because branch protection can name just this one context, the job set can be split or renamed without touching the ruleset. Branch protection itself is configured in GitHub, not in this repository.

Seven jobs also never skip because of an upstream failure: `Plan Go work`, `Build, vet`, `Lint`, `Test (internal/api)`, `Test (everything else)`, `Coverage gate` and `Web (tsc, eslint)`. Each runs, and fails, whenever an upstream job failed or was cancelled, and is skipped only when its area has no work for the diff. GitHub reports a job skipped by its own `if` as success for a required check, so auto-merge does not wait on it.

## Other workflows on a PR

| Workflow | On a PR | Why |
| --- | --- | --- |
| `secret-scan.yml` (gitleaks) | Every PR, commits in the change only | A token leaked in a markdown file is still a leak. Uses the pinned release binary, no Go build |
| `pr-hygiene.yml` | Every PR | Labels, size, description and commit rules in one job |
| `codeql.yml` | No | Not a required check. Runs on pushes to `main` that touch Go or web sources, weekly, and on manual dispatch |
| `dependabot-auto-merge.yml` | Dependabot PRs only (listed as skipped elsewhere) | |
| SonarCloud | Every PR | A GitHub App, not an Actions job: it runs on the vendor's infrastructure, uses no Actions minutes, and is not a merge blocker |

## First-hour smoke test

The install-to-rollback journey is covered by `scripts/test-first-hour.sh`. How to run it and a ready-to-use nightly job are in [First-hour smoke test](/first-hour-smoke-test).

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

Versions are automatic. [release-please](https://github.com/googleapis/release-please) reads the conventional commit messages on `main`, picks the next version, updates `CHANGELOG.md`, and keeps one open pull request titled `chore(main): release <version>`. Merging that PR tags the release, creates the GitHub Release, and dispatches `release.yml`, which builds every binary and publishes to Homebrew and npm.

Releases are paced. While the project is pre-1.0, a `feat` bumps the patch (0.8.1, 0.8.2), not the minor, so versions go deep instead of wide; a minor jump such as 0.8.0 needs the `allow-minor-release` label on the release PR. The `Release pacing` check also fails until `RELEASE_COOLDOWN_HOURS` (repository variable, default 6) have passed since the last release, so two releases cannot land in the same hour. An hourly workflow re-runs the failed check, so an armed auto-merge goes through on its own once the wait ends. The `skip-release-cooldown` label overrides the wait for an urgent fix. `scripts/test-release-pacing.sh` is the self test.

The one manual step is merging that PR. By default release-please uses the built-in `GITHUB_TOKEN`, and GitHub does not run CI on pull requests opened that way, so the required `CI required` check never reports and the PR stays blocked until someone reopens it or merges around the gate.

To make releases hands-off, give release-please a real token. A fine-grained personal access token limited to this repository, with Contents and Pull requests set to read and write, is enough:

```bash
gh secret set RELEASE_PLEASE_TOKEN --repo glincker/levelrail    # paste the token at the prompt
gh variable set AUTO_RELEASE --body true --repo glincker/levelrail
```

With the secret, the release PR runs CI like any other PR, and the tag push starts `release.yml` by itself. With the variable as well, release-please also arms auto-merge on the PR, so a change on `main` ships as a new beta once CI passes. Every merge to `main` updates the same PR, so this releases on every merge. Leave `AUTO_RELEASE` unset to keep merging release PRs by hand, with the token still doing its job of running CI on them.

### Commit and PR standard for releases

`main` only receives squash merges, so the squash commit is the only thing release-please reads.

- **Title**: `type(scope): user-visible outcome`, under 72 characters, one idea. The title becomes the changelog line.
- **Types that ship**: `feat`, `fix`, `perf`, `security`, `docs` appear in the changelog. `refactor`, `test`, `ci`, `chore` are hidden and never trigger a release on their own.
- **Version bump**: pre-1.0, `feat` and `fix` both bump the patch. A minor needs the `allow-minor-release` label on the release PR. Never write `!` or a `BREAKING CHANGE:` footer before 1.0 without an owner decision, because it proposes 1.0.0.
- **Forcing a version**: put `Release-As: x.y.z` in the body of the squash commit, typed into the merge box. Putting it in the PR description does nothing, because the repo squashes with the commit messages, not the PR body.
- **Body**: at most 1 to 3 short lines of why. No code, file lists or test output.
- **One merge per release**: merge feature work to `main` with a `feat` or `fix` title, then let the release PR merge itself once `AUTO_RELEASE` is on (see above). Do not hand edit the manifest, `CHANGELOG.md` or the release PR body. A wrong version is fixed with `Release-As` on the next commit.
- **Release lines**: a `release/*` branch goes to `main` as one squash PR. Never push to it again after it merges, because the push re-creates the deleted branch.
- **Build before tag**: a change to a Dockerfile, `docs/`, or an install script must build the web image stage from a clean `git archive` export before it merges, because the Docker context has no symlink targets outside `web/` and `docs/`.
- **No attribution trailers**: no `Co-Authored-By`, `Claude-Session`, or generated-by lines in commits or PR bodies. The `commit-msg` hook and the PR hygiene check both block them.

## Release gate

`release.yml` has a `release-gate` job between the binary build and everything that publishes (the release assets and both container images wait on it). It runs `scripts/release-gate.sh` with the candidate `levelrail-linux-amd64`, the previous published release's binary (checksum verified against its `checksums.txt`) and the seeded databases from `scripts/upgrade-fixtures/generate.sh`. It fails the release if the schema version goes backwards, if the previous release cannot be replaced by the candidate on the same data (checked through the control plane's own `/readyz`, no Docker), or if the candidate does not refuse a newer database with exit 78. A release with no earlier release to compare against only warns. The `release` job also refuses to publish when `release-manifest.json`, the file the Updates page reads the schema from, is missing from the signed `checksums.txt`. `scripts/test-release-gate.sh` builds two binaries and checks the gate passes the good path and refuses the bad ones; it is not part of PR CI.

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

`publish-npm` publishes `levelrail-cli` plus six platform packages (`levelrail-cli-<os>-<arch>`) using npm trusted publishing, so no npm token is involved. Stable releases take the `latest` dist-tag and prereleases use `beta`. A version already on the registry is skipped, so re-running a partly failed release finishes the rest. The job only runs when the `NPM_PUBLISH_ENABLED` repo variable is `true`.

One-time setup. npm configures a trusted publisher on a package that already exists, so each package is published once by hand first. The script does the whole thing under your own npm login:

```bash
scripts/npm-bootstrap.sh --dry-run    # builds the packages and shows every step, changes nothing
scripts/npm-bootstrap.sh              # publishes, registers the trusted publisher, then enables the job
```

It downloads the newest release's binaries, builds the seven packages, publishes the platform packages first, runs `npm trust github` on each package for `glincker/levelrail`, workflow `release.yml`, environment `npm-publish`, and only then sets `NPM_PUBLISH_ENABLED`. It skips anything already done, so it is safe to re-run after a failure. Publishing and registering both ask for your npm two-factor code.

After that, every release publishes with no token. If a publish fails, use "Re-run failed jobs" on the workflow run; versions already on the registry are skipped.

An `NPM_TOKEN` secret, including one set at the organization level, is not used and can stay as it is for other repositories.

## Caching

Set the repository variable `GOCACHE_BUCKET` (plus `GOCACHE_REGION`,
`GOCACHE_ENDPOINT` for R2, Tigris or MinIO, and the secrets `GOCACHE_KEY_ID`
and `GOCACHE_SECRET`) to switch every Go job to a shared remote build and test
cache through `GOCACHEPROG` ([gobuildcache](https://pkg.go.dev/github.com/richardartoul/gobuildcache)).
A package whose inputs hash the same is a cache hit, build and test result
alike, across PRs, nightly and laptops. Unset, the actions/cache path below
applies. Live Docker lanes should keep `-count=1`.

- Go: `~/.cache/go-build` and `~/go/pkg/mod`, one cache per job kind (build,
  lint, and each test lane group), keyed on the Go version and `go.sum` plus
  `tools/go.sum`. Pushes to `main` save one fresh cache per day, in the job's
  post step so it holds what the job compiled. All test lanes share one cache
  (`go-test-all`, saved by the first lane). PR runs only
  restore, so they cannot churn the repository's 10 GB cache quota. Every
  job that needs this shares `.github/actions/setup-go-cached` instead of
  repeating setup-go plus restore/save inline.
- golangci-lint: the action's own analysis cache, saved on `main` only.
- npm: `actions/setup-node`'s npm cache, keyed on `web/package-lock.json`.
- Dependabot: its PRs go through the same scoping. A GitHub Actions bump touches
  only `.github/workflows/*`, so it runs actionlint and nothing else; an npm bump
  runs the web jobs only; a `tools/` bump runs the release notes generator; a
  `go.mod`, `go.sum` or `kit/go.mod` bump runs every Go lane, because the import
  graph cannot scope a dependency change.
- Release image cache: `release.yml` writes its BuildKit layers with
  `cache-to: type=gha`, and a tag run can only read caches of its own tag, so
  each release leaves about 5 GB that no later run reads and that competes with
  the CI caches for the 10 GB quota.
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

A new push to a PR cancels that PR's previous CI run (`ci.yml`, `secret-scan.yml`,
`pr-hygiene.yml`). The group key includes the event name and, outside pull
requests, the commit SHA, so `merge_group` and `main` runs never share a
group with a PR and are never cancelled; `nightly.yml` has no group.

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

Both scoping scripts run locally against any pair of commits, which is the quickest way to see what a change selects.
`scripts/test-ci-changes.sh` holds the table cases for `ci-changes.sh` (docs-only,
web-only, lockfile-only, script-only, Go-only, `go.mod`, workflow, planner and
embedded-file changes), the shard partition checks and a fake-`gh` check that a
skipped, missing or failed lane fails the aggregator that `CI required` waits on:

```sh
scripts/ci-changes.sh <base-sha> <head-sha>
scripts/ci-go-plan.sh <base-sha> <head-sha>
```

## Measured timings

Baseline: the last 60 `ci.yml` runs (48 of them on pull requests). Median
wall time per job, from the run and step timestamps.

| Job | Median | p90 |
| --- | --- | --- |
| Whole PR run | 491 s | 622 s |
| `Test (api-1..4)` (internal/api shards) | about 390 s | about 450 s |
| `Test (e2e)` / `e2e-fleet` / `e2e-reconcile` | 382 s / 468 s / 475 s | 412 s / 490 s / 525 s |
| `Test (rest)` / `Test (docker)` | 223 s / 182 s | 283 s / 217 s |
| `Build, vet` (planner table tests about 128 s of it) | 221 s | 266 s |
| `Lint` | 183 s | 237 s |
| `Web (tsc, eslint)` (`vitest related` has rare outliers) | 152 s | 375 s |
| `Plan Go work`, `Coverage gate`, `CI required` | 49 s, 12 s, 3 s | |

The first job starts 2 to 57 s after the run is created. Go build and
module caches hit on PR runs. The tsc build-info cache never hits, because
`tsc -b` re-checks the whole project on every run (about 31 s) and
`web/tsconfig.app.json` sets no `incremental`. Enabling it is a tsconfig change
that needs maintainer approval; the cache save step would then need to allow
PR runs as well.

Local hooks, one small sample diff each on a warm cache (seconds):

| Stage | Before | After |
| --- | --- | --- |
| pre-commit, Go file only | 2.3 | 0.7 (warm cache both runs, so not a gain of this change) |
| pre-commit, one web file | 35.3 | 29.5 (eslint and prettier now run beside `tsc -b`) |
| pre-push, Go file (`internal/brand`) | 59.4 | 33.8 |
| pre-push, web file only | 21.0 | 1.7 (`go build ./...` skipped when nothing Go-affecting changed) |
| internal/api narrowing (`affected-api-tests.sh`) | 36 | 13 |

A fresh worktree has no `web/src/routeTree.gen.ts` (gitignored, written by
`vite build`), so pre-commit used to fail on every web change there. It now
runs `vite build` once when the file is missing, about 20 s.

pre-push bounds every `go test` it runs with `-timeout`, default `30m`,
overridable with `LEVELRAIL_PUSH_TEST_TIMEOUT`. The default `go test`
timeout of 10 minutes is shorter than a whole internal/api run.

`scripts/test-fast.sh` runs `go test -short` for internal/api in up to 6
shards from one binary, with CI's weights and quarantine skips (`-r REGEX`
narrows tests). pre-push uses it. Failures print test names and keep the
shard logs. Test builds hash passwords at bcrypt's minimum cost: over 10
minutes serial became about 3.

## Expected effect of the lane changes

Estimates from the artifacts of three recent runs, to be compared with the table
above after the first runs on the new layout.

| Change | Expected | Basis |
| --- | --- | --- |
| Weighted api shards | api lanes lose about 25 s of test time (the slowest shard's summed test time 303 s to 276 s; shards were 232 to 303 s) | per-test means of 3 runs, packed greedily |
| `e2e-reconcile` split in two | the reconcile lane drops from about 480 s to about 280 s; with `e2e-fleet` (about 410 s) now the longest e2e lane, a run that plans the fleet lanes saves roughly 65 s of wall time | per-test times of the reconcile package |
| Planner tests on planner-only changes | no wall effect; a change to the planner tests alone no longer skips them | `ci-changes.sh` planner output |

The api shard balance is noisy run to run (a shard's summed test time varied by
up to 30% between runs with the same assignment), so the gain from weights is
smaller than the spread.

## Runners

Every job reads `runs-on` from the `CI_RUNNER` repository variable and falls
back to `ubuntu-latest`, so trying a faster provider (Blacksmith, Namespace,
RunsOn) is a variable change, not a workflow edit.

## Blast radius

`scripts/check-fanin.sh` prints how many packages transitively depend on each
package and fails when one passes its budget in `scripts/fanin-budget.txt`.
Today `internal/store` reaches ~60 packages through `internal/spec`, so most
store edits select most of the tree. Lowering those numbers (consumer-side
interfaces, moving shared types to leaf packages) is the lasting fix for
"one edit runs 1000 tests".

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
