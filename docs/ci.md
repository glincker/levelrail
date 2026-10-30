---
title: "CI"
description: How the GitHub Actions checks on a pull request are chosen, what they cost, and which ones branch protection relies on.
"og:url": "https://docs.levelrail.com/ci"
---

How the GitHub Actions checks on a pull request are chosen and which ones gate a merge. This page is for contributors to Levelrail itself. To deploy your own apps from CI, see [Use any CI](/use-any-ci).

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
every `Test (*)` job, `Web (tsc, eslint)`, `install.sh`),
scoped against `github.event.before` and falling back to `--full` when that
SHA is missing or unresolvable. The merge queue is gone, so this is the only
post-merge verification of the merged tree. Docs-only pushes never trigger
`ci.yml` at all (`paths-ignore` on the `push` trigger: `**/*.md`, `adr/**`,
`LICENSE`), and an area with no changed files skips as on a PR.
`nightly.yml` is the safety net for anything the scoping under-selects. Monday
to Saturday it runs shuffled, no `-short`, no `-race` lanes scoped to the changes
since the last all-green run, so a lane that failed or timed out is retried until
it passes. Sunday and a manual dispatch run the full sweep with `-race`: the api
lanes, the docker lane and the `race-a` / `race-b` halves of the packages that
start goroutines or use `sync`, `atomic` or channels
(`scripts/go-test-groups.sh race`). The `rest` packages split on
`scripts/ci-heavy-packages.txt` so no lane nears its timeout, and the docker
lane pre-pulls its upstream images with retries. One summary table and one
`nightly-failure` issue report the run (see "Nightly and flaky tests").

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

The docker lane runs in the Sunday and manual full sweep (`NIGHTLY_LIVE_ALWAYS`
keeps it in that matrix even with no commits), so an upstream image change is
caught within a week. Daily runs include it only when the diff reaches it.

Run the full set on a developer machine with one command:

```sh
scripts/live-tests.sh                       # all of test/e2e and test/e2e/reconcile
scripts/live-tests.sh -run TestBreakGlass   # one suite
```

The fleet test deploys `LEVELRAIL_FLEET_PARALLEL` templates at once (default
3, `1` for strictly sequential). Each template gets its own app id, network
and volumes, and teardown asserts nothing leaked per template.

Coverage never blocks or fails a PR. `Coverage gate` runs on push to `main`
only, is `continue-on-error`, and is not a need of `CI required`. On those
runs a full plan checks the 70% aggregate for `internal/`, a scoped plan the
changed-line gate at 50%. The `kit/` coverage gates follow the same rule. The
Codecov statuses are informational and there is no PR comment (`codecov.yml`).

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

`CI required` is the aggregator and the one check meant to gate a merge. It always runs and needs `changes`, `plan`, `build-vet`, `lint`, `kit`, the Go test jobs, `web-check`, `install-sh` and the macOS and Windows CLI installer jobs. It fails if change detection did not succeed or any job it needs failed or was cancelled, and a job that was skipped because its area had no work counts as passing. Because branch protection can name just this one context, the job set can be split or renamed without touching the ruleset. Branch protection itself is configured in GitHub, not in this repository.

Six jobs also never skip because of an upstream failure: `Plan Go work`, `Build, vet`, `Lint`, `Test (internal/api)`, `Test (everything else)` and `Web (tsc, eslint)`. Each runs, and fails, whenever an upstream job failed or was cancelled, and is skipped only when its area has no work for the diff. GitHub reports a job skipped by its own `if` as success for a required check, so auto-merge does not wait on it.

### Draft PRs

A draft PR runs only the cheap checks: `Detect changed areas`, `Plan Go work`,
`Build, vet`, `Lint`, `Web (tsc, eslint)` and the other workflows (secret scan,
hygiene). The heavy jobs are skipped: every `Test (*)` lane, `Test (internal/api)`,
`Test (everything else)`, the quarantine lane, `Web tests`, `Kit`, the Terraform
provider job and the installer jobs. `CI required` reports success for those
draft skips only. Mark the PR ready for review to run the full suite: the
workflow listens for `ready_for_review`, so the conversion starts a full run.
Set the repo variable `CI_ON_DRAFTS=true` to run everything on drafts too.

On a ready PR, `CI required` also fails when the planner says a job is needed
(Go tests planned, kit, provider, vitest or installer changes) and that job was
skipped, so a non-draft PR cannot merge without the heavy lanes. Branch
protection should keep requiring `CI required`.

## Other workflows on a PR

| Workflow | On a PR | Why |
| --- | --- | --- |
| `secret-scan.yml` (gitleaks) | Every PR, commits in the change only | A token leaked in a markdown file is still a leak. Uses the pinned release binary, no Go build |
| `pr-hygiene.yml` | Every PR | Labels, size, description and commit rules in one job |
| `codeql.yml` | No | Not a required check. Runs weekly, on release tags and on manual dispatch (a push to `main` cost about 17 runner-minutes, so it no longer triggers one) |
| `dependabot-auto-merge.yml` | Dependabot PRs only (listed as skipped elsewhere) | |
| `pr-security.yml` | PRs touching workflows or dependency manifests | zizmor (advisory) and dependency review (fails only on a newly added high or critical advisory). Neither is in `CI required` |
| SonarCloud | No | `sonar.yml` runs on `main` and `release/**` pushes, weekly and on dispatch. Advisory, see [Quality and security scans](#quality-and-security-scans) |

## Quality and security scans

All of these are advisory: `continue-on-error`, never in the `needs` of
`CI required`, and a missing secret skips the step with a notice instead of
failing.

| Scan | Workflow | When | Output |
| --- | --- | --- | --- |
| SonarCloud | `sonar.yml` | Push to `main` and `release/**`, Mondays, dispatch. Never on a PR | SonarCloud dashboard. Needs `SONAR_TOKEN`. Go coverage (`go test -short ./internal/...`) is imported everywhere except release branch pushes |
| Codecov | `ci.yml`, `Coverage gate` | Push to `main` only (`ci.yml` has no `release/**` push trigger, the guard also covers it) | Informational statuses, no PR comment. Needs `CODECOV_TOKEN`. Config in `codecov.yml` |
| govulncheck | `security-scheduled.yml` | Mondays, release tags, dispatch | Reachable findings in the job summary, SARIF to code scanning |
| OpenSSF Scorecard | `security-scheduled.yml` | Mondays, dispatch | SARIF to code scanning |
| OSV-Scanner | `security-scheduled.yml` | Mondays, dispatch | SARIF to code scanning (Go and npm lockfiles) |
| CodeQL | `codeql.yml` | Wednesdays, release tags, dispatch | Security tab |
| zizmor | `pr-security.yml` | PRs touching `.github/` | Job log. actionlint stays in `Lint` and is the gate |
| Dependency review | `pr-security.yml` | PRs touching a lockfile or `go.mod` | Fails on a new high or critical advisory only |
| gitleaks | `secret-scan.yml` | Every PR and `main` push | The one scanner that blocks |

SonarCloud is off by default: the `SonarCloud scan` job in `sonar.yml` only
runs when the repository variable `SONAR_ENABLED` is `true` (repository
settings, Secrets and variables, Variables). Set it to re-enable the scan;
otherwise the job shows as skipped. `sonar.yml` is not part of `CI required`,
so a skipped scan blocks nothing. The `SonarCloud Code Analysis` status check
comes from the Sonar GitHub app, not from this repo, and the owner must switch
off automatic analysis in the SonarCloud project settings to stop it. When the
scan is re-enabled, automatic analysis must stay off, because SonarCloud
rejects a CI scan while it is on.
Web coverage is not uploaded yet: vitest has no coverage provider in
`web/package.json`.

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

## Release branch pushes

`ci.yml` also runs on `push` to `release/**`, so each push to the release line
gets the same impact-scoped run a PR gets. Concurrency groups a release branch by
ref and cancels the older run, so a burst of pushes keeps only the newest.
Because a cancelled run never reports its commit, `changes` diffs a release push
against its merge-base with `main` (the cumulative release diff), not against
`github.event.before`. Other pushes use `before` and fall back to the same
merge-base when it is the zero SHA or no longer reachable (a force push).
Public repository minutes are free, but a push burst still queues runners and
delays the PR checks, so batch pushes. `paths-ignore` for Markdown and `adr/**`
stays: required checks only matter on PRs, which have no path filter.
`coverage-gate` and the cache saves also run for these pushes. The draft logic
is unchanged because it reads the pull request payload.

## Concurrency and merge queue

A new push to a PR cancels that PR's previous CI run (`ci.yml`, `secret-scan.yml`,
`pr-hygiene.yml`, `pr-security.yml`). `sonar.yml`, `security-scheduled.yml` and
`codeql.yml` cancel a superseded run of the same ref, and `dependabot-auto-merge.yml`
of the same PR. The group key includes the event name and, outside pull
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

Measured 2026-10-24 from the last 60 runs of each workflow (`gh api`).
Runs per week extrapolates from that window, so bursty workflows read high.
Failed and cancelled counts are out of 60.

| Workflow | Events | Median / p90 | Runs per week | Failed / cancelled |
| --- | --- | --- | --- | --- |
| `ci.yml` | PR 51, push 9 | 421 s / 840 s | about 240 (1.8 day window) | 13 / 23 |
| `codeql.yml` (go 764 s, js 213 s) | push 58, schedule 2 | 634 s / 1218 s | 32 | 0 / 3 |
| `release.yml` | tag push, dispatch | 426 s / 971 s | 20 | 26 / 0 (most before the tag-only trigger) |
| `nightly.yml` | schedule | 2631 s / 4521 s | 7 | 54 / 1 |
| `secret-scan`, `pr-hygiene`, `glinr-bot` | PR | 15 s, 20 s, 36 s | 340 to 500 | 0 / 0, 1, 20 |
| `release-pacing`, `release-please`, `flake-report` | schedule, push, workflow_run | 7 s to 37 s | 37 to 171 | 2 / 0 |
| Housekeeping (`stale`, `lock-closed`, `backup-tags`, `branch-cleanup`, `labels`) | schedule, PR | under 15 s | under 10, except `branch-cleanup` on every PR close | 0 / 0 |

Where the runner minutes go, per `ci.yml` run (about 44 runner-minutes, from
the jobs of 30 runs):

| Job | Median | p90 | Runner-min per run |
| --- | --- | --- | --- |
| `Web tests (k/4)` (full vitest, on 2 of 3 recent runs because `ci.yml` itself changed) | 347 s | 384 s | 14.1 |
| `Test (api-1..4)` | 378 s | 495 s | 7.9 |
| `Test (rest)`, `Build, vet`, `Lint` | 490 s, 452 s, 346 s | 552 s, 464 s, 388 s | 2.8, 2.5, 1.9 |
| `Test (e2e)`, `e2e-reconcile`, `e2e-fleet`, `docker` | 370 s, 305 s, 518 s, 349 s | 771 s, 492 s, 584 s, 492 s | 2.4, 1.8, 1.1, 1.9 |
| `Web (tsc, eslint)` | 184 s | 242 s | 2.2 |

Other sinks: `codeql.yml` cost 17.6 runner-minutes on every push to `main`
(14 for the Go job), about 4.5 pushes a day. `release.yml` costs about 35
runner-minutes per tag, mostly the four image builds. Flaky lanes are
`Test (e2e-reconcile-2)` (4 of 5 runs failed) and the `api` shards (8 of 40).

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
timeout of 10 minutes is shorter than a whole internal/api run: 3,940 tests
take about 7 minutes serial on a loaded laptop (about 0.1 s each; only 18 tests
pass 1 s, 69 s together), so there is no single slow test to fix. CI splits the
package into the four `api-N` shards, each well under its `-timeout=10m`, and
`test/e2e/reconcile` is a docker-lane package (live catalog deploys that pull images
when a daemon is up, so a local run can pass 10 minutes) and CI gives it `-timeout=27m`;
`scripts/verify.sh` passes `-timeout` (default 45m, `LEVELRAIL_VERIFY_TEST_TIMEOUT`).

`scripts/test-fast.sh` runs `go test -short` for internal/api in shards
from one binary, with CI's weights and quarantine skips (`-r REGEX`
narrows tests). pre-push uses it for every internal/api run, in
`min(4, cores)` shards, overridable with `LEVELRAIL_PREPUSH_API_SHARDS`
(`1` keeps one `go test` process in the `affected` and full lanes). With
coverage on, internal/api stays in the single `go test` run. Each shard's
output is captured; a failure prints only the failing tests with their
output lines, and the shard logs are kept. The run ends with per-shard
seconds so a skewed split shows up. `scripts/test-api-shards.sh` checks that
the shard split covers every internal/api test from `go test -list` exactly
once. Test builds hash passwords at bcrypt's minimum cost: over 10
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

## Nightly and flaky tests

What the numbers said (60 days to 2026-10-24, `nightly.yml` on `main`): 69 runs,
8 green, 60 red, 1 cancelled, so a red nightly carried no signal. Median wall
time 43 minutes (max 75). About 190 billable job minutes a night, 326 on a
Sunday with `-race` (roughly 1,450 a week). The `race` and `docker` lanes were
about 120 of those minutes and hit their 60 minute job limit most nights.
`template-smoke.yml` and `vps-smoke.yml` are near idle (1 and 0 runs in that
window) and cost nothing worth tuning.

Red nights were mostly a handful of real, unfixed regressions seen again every
night (dashboard budget test 12 nights, live log blue-green test 9, the
`Controller_Reconcile_Live` and ExecTTY live tests for about two weeks),
plus lane timeouts (about 15 nights) and registry throttling (429 from
`public.ecr.aws`). Only one test was a true timing flake
(`TestAuthLibSessionsUnknownUserTimingMatchesWrongPassword`).

Decision: keep a daily run, make it cheap, and move the expensive lanes to a
weekly sweep.

| Run | Cron | Lanes |
| --- | --- | --- |
| Daily | Monday to Saturday 04:00 UTC | Scoped to changes since the last all-green run: `api-1..4` (shuffled, no `-race`), `rest-heavy`, `rest-light`, kit, govulncheck, install.sh |
| Weekly sweep | Sunday 04:00 UTC and manual dispatch | Everything above with `-race` where it applies, plus `docker` and the `race-a` / `race-b` halves of the concurrent packages, unscoped, with 75 minute test timeouts |

Estimated cost is about 55 job minutes on a busy daily run and 300 to 350 on
the Sunday sweep, about 650 a week, down from about 1,450.

How it reports without turning `main` red:

- Every test step has `continue-on-error` and writes `status-<lane>` as an
  artifact, so the run itself stays green. It is a scheduled workflow, never a
  required check (`CI required` is the only one).
- The `summary` job writes one table to the job summary, opens or updates a
  single `nightly-failure` issue while any lane is red, and closes it when
  green. Flake sweep and quarantine lanes are advisory and never count.
- An all-green run uploads a `nightly-green` marker. The next daily run diffs
  against that commit, so a failed lane is retried until it passes.

Quarantine: `.github/flaky-tests.txt` entries (with an issue link, 14 day limit)
are skipped by every lane and run alone, non-blocking, in the `quarantined` job
(`QUARANTINE=only`). Prefer fixing: inject a clock or resolver, use
`t.TempDir` and `t.Setenv`, wait on a condition instead of sleeping.

Test hygiene found this way: a test that saves a URL with a public hostname
must not resolve DNS (set `netguard.AllowPrivateEnv` through a helper), a live
test that parses an SSE stream must skip `event: status` frames, and a timing
comparison should use the fastest of several runs, not the median.

## Local dev loop for parallel agents

- Pre-commit lint runs with `--allow-parallel-runners` and one cache shared by
  all worktrees (`LEVELRAIL_LINT_CACHE` overrides the path). If a binary still
  reports the single-run lock, the hook retries for `APP_LINT_LOCK_WAIT_SECONDS`
  (default 300), then fails with a message. Lint is never skipped silently.
- Pre-push clears the `GIT_*` variables git exports to hooks, so tests that
  create their own repositories behave the same in the foreground as in the
  background.
- Pre-push runs `go run ./scripts/gen-api-reference -check` when a routes file
  or a generated API output changed, and prints the regenerate command if stale.
- `scripts/gate-api.sh [-r regex]` runs `./internal/api -short` in CI's lanes
  (`GATE_API_LANES`, default 4) with `GATE_API_CONCURRENCY` lanes at a time,
  printing only failures and a summary line.
- `scripts/clean-agent-worktrees.sh [--apply]` lists, and with `--apply`
  removes, linked worktrees that are clean and fully contained in a remote
  branch. It never touches the primary worktree, the current one, or any
  dirty or unpushed tree.

## Pre-integrate gate

`scripts/pre-integrate.sh [--web] [--base REF]` is the one local gate to run before
a branch is squash-merged into the release branch or the release PR is opened. It
stops at the first failure and always prints a summary with each step's time. The
base defaults to the closest `origin/release/*` merge-base, like the pre-push hook.
Typical times on a warm laptop (no `--web`) are in the last column.

| Step | Runs when | Time |
| --- | --- | --- |
| gofmt on changed Go files | Go files changed | 1s |
| `scripts/code-gen-check.sh` | always | 4s |
| `scripts/check-migration-versions.sh` plus `--against-remotes` | always; fetches origin and reads every `origin/*` branch | about 1 min |
| `scripts/check-fanin.sh` | always | 3s |
| `scripts/check-dashes.sh` | always | 1s |
| i18n parity (`localeParity.test.ts`) | `web/src/locales/` changed | 10s |
| API guards (`./internal/api`) | `internal/api`, route or openapi files changed | 30s to 2 min |
| CLI guards (`TestCommandTree`, `TestHelpTextCoversEveryFlag`) | same as above | 20s |
| `go vet` on changed packages | Go files changed | 10s to 1 min |
| `tsc -b` | `--web` | 1 to 3 min |
| bundle size and report | `--web`; fails on any FAIL line | 1 to 2 min |

Knobs: `PRE_INTEGRATE_NODE_PATH`, `PRE_INTEGRATE_NODE_HEAP`, `PRE_INTEGRATE_TAIL`,
`PRE_INTEGRATE_MIGRATION_STRICT=1` (fail instead of warn when another remote branch
uses the same migration number under a different file name), `LEVELRAIL_PUSH_BASE`.
`scripts/next-migration.sh` prints the next free number across the working tree and
all `origin/*` branches; `scripts/test-next-migration.sh` checks it on a temp repo.
