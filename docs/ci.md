---
description: How the GitHub Actions checks on a pull request are chosen, what they cost, and which ones branch protection relies on.
---

# CI

How the GitHub Actions checks on a pull request are chosen, what they cost,
and which ones branch protection relies on.

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
| `.golangci.yml` | Full lint |
| `*.go` | That package, plus its dependents; lint for that package |
| Non-Go file inside a Go package (embedded schema, SQL, `testdata/`) | That package |
| File outside any package (`docs/`, root files, `proto/`) | Packages whose Go files name it in a string literal, for example `internal/api` for `docs/api-reference.md` |
| `web/` | Web job; config, lockfile or test setup changes run the full vitest suite |
| `docs/*.md` | Web bundle build and the help page tests |
| `install.sh`, `scripts/test-install-sh.sh`, `packaging/` | install.sh end to end on Ubuntu and Debian |
| Other `.github/workflows/*` | actionlint on the changed files |
| `.github/flaky-tests.txt` | The packages named in the changed lines, plus the quarantine lane |
| Anything else (`adr/`, `LICENSE`, other `.github/` files) | Nothing beyond the always-on checks |

`scripts/ci-go-plan.sh <base> [head]` (inside **Build, vet**) turns the Go
side into packages and test lanes through `scripts/affected-go-packages.sh`,
the same dependency-graph walk the pre-push hook uses. A change only a
package's own tests can see (`_test.go`, `testdata/`, a doc a test reads)
selects that package alone, not its dependents. Lanes:

- `internal/api` in four shards by test name whenever it is affected. It is
  the slowest package by far, and most backend changes reach it.
- Up to `CI_SMALL_LANE_MAX` (default 8) other affected packages share one
  `rest` lane. Above that, Docker-backed packages get their own `docker`
  lane with bounded `-p` and the rest share `rest`.
- No lane at all when nothing Go-related is affected.

`test/e2e` is split from `test/e2e/reconcile` along the one dependency that
mattered: only the former constructs a live `api.Router`. Since
`internal/api` is the package most PRs touch, a one-file fix there used to
select the whole 36-file suite through the test-import walk; now it selects
only the ~18 files that actually exercise the HTTP API, and
`test/e2e/reconcile`'s ~18 reconciler/Docker-only tests run only when a PR's
diff genuinely reaches something they import (see
`test/e2e/testenv`'s doc comment for the shared, `internal/api`-free
helpers both packages use).

The coverage gate follows the plan: a full run checks the 70% aggregate
for `internal/`, a scoped run checks the changed-line gate only, at a
lower 50% bar (a partial profile makes the aggregate meaningless, same as the
pre-push hook).

## Required checks

Branch protection on `main` currently requires these six contexts:

- `Build, vet`
- `Lint`
- `Test (internal/api)`
- `Test (everything else)`
- `Coverage gate`
- `Web (tsc, eslint)`

Each of these jobs runs, and fails, whenever an upstream job failed or was
cancelled. It is skipped only when its area has no work for the diff, and
GitHub reports a job skipped by its own `if` condition as success for a
required check, so auto-merge does not wait on it.

**CI required** is the aggregator: it always runs, needs every gating job,
and fails if change detection did not succeed or any job it needs failed or
was cancelled. Branch protection requires only this one context (confirmed
against the live ruleset), not the six job names individually, so it stays
stable if jobs are later split or renamed.

## Other workflows on a PR

| Workflow | On a PR | Why |
| --- | --- | --- |
| `secret-scan.yml` (gitleaks) | Every PR, commits in the change only | A token leaked in a markdown file is still a leak. Uses the pinned release binary, no Go build |
| `pr-hygiene.yml` | Every PR | Labels, size, description and commit rules in one job |
| `codeql.yml` | No | Not a required check. Runs on pushes to `main` that touch Go or web sources, weekly, and on manual dispatch |
| `dependabot-auto-merge.yml` | Dependabot PRs only (listed as skipped elsewhere) | |
| SonarCloud, Greptile | Every PR | GitHub Apps, not Actions: they run on the vendor's infrastructure and use no Actions minutes |

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

`publish-npm` publishes `levelrail-cli` plus six platform packages (`levelrail-cli-<os>-<arch>`) using npm trusted publishing, so no npm token is involved. A prerelease goes out under the `beta` dist-tag, and a version already on the registry is skipped, so re-running a partly failed release finishes the rest. The job only runs when the `NPM_PUBLISH_ENABLED` repo variable is `true`.

One-time setup, because npm configures a trusted publisher on a package that already exists:

1. Publish each of the seven packages once by hand so it exists. Build them with `scripts/build-npm-packages.sh <tag> <dist-dir> <out-dir>`, then run `npm publish --access public --tag beta` inside each directory while logged in with two-factor authentication. Do the platform packages first and `levelrail-cli` last.
2. On npmjs.com, open each package, then Settings, then Trusted publishing, and choose GitHub Actions with organization `glincker`, repository `levelrail`, workflow `release.yml`, and environment `npm-publish`.
3. Turn the job on: `gh variable set NPM_PUBLISH_ENABLED --body true --repo glincker/levelrail`.

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

## Concurrency and merge queue

A new push to a PR cancels that PR's previous CI run. Runs on `main` are
never cancelled.

There is no merge queue on `main` now, so a PR merges as soon as its own
`CI required` check passes and the `push` run on `main` is the only
verification of the merged tree. That run does everything a PR run does,
scoped by the same impact detection, so a change that was fine alone but
breaks against a newer `main` is caught the same day, not at the nightly
sweep.

`ci.yml` still listens for `merge_group`, with the same base-commit scoping
as a PR, so re-enabling a queue needs only the branch protection setting.
If a queue returns, the `push` run duplicates the `merge_group` run on the
same tree; restore a `github.event_name != 'push'` guard on the heavy jobs
at that point (PR #939 did exactly this while a queue existed).

`Build, vet` also refreshes the daily Go build cache on `push`
(the cache `save` flag, true only for push events), which PR runs depend on
staying warm. `codeql.yml` and `secret-scan.yml` are separate workflow
files with their own `push` triggers, unaffected by anything in `ci.yml`.

## Measured cost before this change

The last 30 merged PRs (#726 to #762), every push, from the Actions API.
The repository is public, so Actions minutes are free: "billed" below is
what a private repository would pay (each job rounded up to a whole
minute), which is also a fair proxy for runner time and queueing.

| Workflow | Job | Runs | Executed | Skipped | Job minutes | Billed minutes | Share |
| --- | --- | ---: | ---: | ---: | ---: | ---: | ---: |
| CI | Test (api-2) | 46 | 46 | 0 | 302 | 327 | 14.5% |
| CI | Test (api-1) | 46 | 46 | 0 | 299 | 321 | 14.2% |
| CI | Web (tsc, eslint) | 50 | 37 | 13 | 215 | 234 | 10.3% |
| CI | Test (docker) | 46 | 46 | 0 | 215 | 232 | 10.3% |
| CI | Test (rest) | 46 | 46 | 0 | 205 | 228 | 10.1% |
| codeql | analyze (go) | 47 | 47 | 0 | 118 | 141 | 6.2% |
| CI | Lint | 50 | 46 | 4 | 74 | 98 | 4.3% |
| codeql | analyze (javascript-typescript) | 47 | 47 | 0 | 57 | 91 | 4.0% |
| CI | Test (api-3) | 14 | 14 | 0 | 74 | 79 | 3.5% |
| CI | Build, vet | 50 | 46 | 4 | 43 | 53 | 2.3% |
| PR hygiene | PR hygiene | 51 | 51 | 0 | 7 | 51 | 2.3% |
| CI | Detect changed areas | 50 | 50 | 0 | 6 | 50 | 2.2% |
| CI | Migration version check | 50 | 50 | 0 | 5 | 50 | 2.2% |
| CI | CI required | 50 | 50 | 0 | 3 | 50 | 2.2% |
| secret-scan | gitleaks | 48 | 48 | 0 | 27 | 48 | 2.1% |
| CI | Test (quarantined, non-blocking) | 50 | 46 | 4 | 22 | 46 | 2.0% |
| CI | Test (everything else) | 50 | 46 | 4 | 5 | 46 | 2.0% |
| CI | Test (internal/api) | 50 | 46 | 4 | 5 | 46 | 2.0% |
| CI | Coverage gate | 50 | 39 | 11 | 16 | 32 | 1.4% |
| Branch cleanup | Delete merged PR branch | 30 | 30 | 0 | 2 | 30 | 1.3% |
| CI | install.sh (both images) | 4 | 4 | 0 | 6 | 8 | 0.4% |
| Dependabot auto-merge | auto-merge | 48 | 0 | 48 | 0 | 0 | 0% |

Total: 2261 billed minutes (1706 job minutes) across 50 CI runs.

Where it went:

- **Go test lanes, 52%.** Every Go change ran every package, including
  PRs that touched one CLI file. The three `internal/api` shards alone
  were 32%.
- **Web, 10%.** vitest was 334 of the job's roughly 390 seconds, and it
  ran the whole suite for any web change.
- **CodeQL, 10%.** Both languages on every PR touching Go or web.
- **Small fixed jobs, about 15%.** Nine jobs of a few seconds each, each
  billed a full minute, on every push, whether the PR needed them or not.

Checks listed per PR (Actions jobs, plus SonarCloud, Greptile and CodeQL
from GitHub Apps): about 25 on a Go PR (18 to 20 of them doing work), 22 on
a web-only PR (9 doing work), 20 on a docs-only PR (6 doing work).

## Estimated cost after

The same 30 PRs replayed through `scripts/ci-changes.sh` and
`scripts/ci-go-plan.sh` (final push of each PR), with each job costed from
the measured step timings above:

| PR kind | PRs | Before: checks listed / doing work | Before: billed min | After: checks listed / doing work | After: billed min |
| --- | ---: | --- | ---: | --- | ---: |
| Go only | 11 | 22.4 / 18.5 | 45.5 | 17.8 / 13.3 | 32.5 |
| Go and web | 15 | 22.3 / 19.4 | 54.1 | 18.9 / 15.0 | 43.3 |
| Web only | 3 | 19.0 / 9.0 | 17.3 | 15.0 / 5.0 | 9.7 |
| Docs only | 1 | 17.0 / 6.0 | 6.0 | 15.0 / 5.0 | 6.0 |

Add two app checks (SonarCloud, Greptile) to the listed counts after, three
(plus CodeQL) before. Across the 30 PRs: 1370 billed minutes before, about
1040 after, roughly 24% less, most of it on PRs that do not reach
`internal/api`. A Go PR that does reach it (most backend work) still pays
for its shards (four as of the latest sharding change), about 21 of its
minutes.

## Dry run against real PR diffs

`git diff <merge commit>^ <merge commit>` of merged PRs, fed through both
scripts on the current tree:

| PR | What changed | Files | Go scope | Test lanes (packages) | Lint | Web | Other | Jobs that do work |
| --- | --- | ---: | --- | --- | --- | --- | --- | --- |
| #756 | docs only | 1 | none | none | none | docs build | | changes, Web, CI required |
| #750 | web only | 20 | none | none | none | full, full vitest (config) | | changes, Web, CI required |
| #703 | npm lockfile bump | 2 | none | none | none | full, full vitest | | changes, Web, CI required |
| #758 | `internal/mcptools` test, a script, docs | 4 | 1 package | rest (1) | 1 dir | docs build | | changes, Build, Lint, rest lane, Test (everything else), Coverage, Web, CI required |
| #762 | CLI only, docs | 6 | 1 package | rest (1) | 1 dir | docs build | | same as #758 |
| #735 | `internal/api` handlers | 3 | 3 packages | api x3, rest (2) | 1 dir | none | | changes, Build, Lint, 4 lanes, both Test aggregators, Coverage, CI required |
| #761 | new package plus API, CLI, MCP | 62 | 11 packages | api x3, docker (2), rest (8) | 5 dirs | docs build | | changes, Build, Lint, 5 lanes, both aggregators, Coverage, Web, CI required |
| #736 | migration, `go.mod`, web | 37 | all | api x3, docker (15), rest (67) | full | full, vitest related | | everything except install.sh |
| #707 | `go.mod` bump | 2 | all | api x3, docker (15), rest (67) | full | none | | everything Go, no Web |
| #666 | a new non-CI workflow | 1 | none | none | none | none | actionlint | changes, Lint (actionlint only), CI required |
| #687 | actions bump incl. `ci.yml` | 5 | all | api x3, docker (15), rest (67) | full | full | install.sh, actionlint | everything (pipeline changed) |
| #619 | `install.sh` only | 1 | none | none | none | none | install.sh x2 | changes, install.sh x2, CI required |
| #680 | `.github/dependabot.yml` | 1 | none | none | none | none | | changes, CI required |

Secret scan and PR hygiene run on all of them. Reproduce any row with:

```sh
scripts/ci-changes.sh <sha>^ <sha>
scripts/ci-go-plan.sh <sha>^ <sha>
```

## Tuning

- `CI_SMALL_LANE_MAX`: the package count at or under which non-api
  packages share one lane (default 8), read by `scripts/ci-go-plan.sh`.
- To force the full suite on a PR, change `.github/workflows/ci.yml` or one
  of the pipeline scripts, or run the nightly workflow on the branch with
  `workflow_dispatch`.
- The docker/rest lane's `go test -timeout` (27m, job `timeout-minutes: 32`)
  has headroom above `test/e2e`'s own measured cost, not a tight fit:
  `test/e2e` runs a fixed ~16-template fleet plus a growing set of
  per-batch catalog live tests sequentially in one binary (no
  `t.Parallel()`), so its floor rises independent of any one PR's diff
  size. #925 and #912 both hit the old 18m/22m budget this way, not from
  a real hang. Raising the ceiling again later means the floor has grown
  further; parallelizing those subtests (independent Docker networks per
  template, should be safe) is the real fix but is a bigger, unverified
  change not made here.
