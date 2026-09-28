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

`main` pushes and the nightly run are not scoped: every push to `main` runs
the whole suite and the aggregate coverage gate, and `nightly.yml` runs the
full `-race`, no `-short` sweep. Those are the safety net for anything the
PR-time scoping under-selects.

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

- `internal/api` in three shards by test name whenever it is affected. It is
  the slowest package by far, and most backend changes reach it.
- Up to `CI_SMALL_LANE_MAX` (default 8) other affected packages share one
  `rest` lane. Above that, Docker-backed packages get their own `docker`
  lane with bounded `-p` and the rest share `rest`.
- No lane at all when nothing Go-related is affected.

The coverage gate follows the plan: a full run checks the 70% aggregate for
`internal/` and the changed-line gate, a scoped run checks the changed-line
gate only (a partial profile makes the aggregate meaningless, same as the
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
was cancelled. The recommended setup is to require only that one check,
which also keeps branch protection stable if jobs are later split or
renamed. This is not applied automatically. To switch (owner action, not
run by CI):

```sh
gh api -X PATCH repos/glincker/levelrail/branches/main/protection/required_status_checks \
  --input - <<'EOF'
{"strict": false, "checks": [{"context": "CI required", "app_id": 15368}]}
EOF
```

Keep the six job names unchanged until the switch is confirmed, so both
configurations work during the transition.

## Other workflows on a PR

| Workflow | On a PR | Why |
| --- | --- | --- |
| `secret-scan.yml` (gitleaks) | Every PR, commits in the change only | A token leaked in a markdown file is still a leak. Uses the pinned release binary, no Go build |
| `pr-hygiene.yml` | Every PR | Labels, size, description and commit rules in one job |
| `codeql.yml` | No | Not a required check. Runs on pushes to `main` that touch Go or web sources, weekly, and on manual dispatch |
| `dependabot-auto-merge.yml` | Dependabot PRs only (listed as skipped elsewhere) | |
| SonarCloud, Greptile | Every PR | GitHub Apps, not Actions: they run on the vendor's infrastructure and use no Actions minutes |

## Caching

- Go: `~/.cache/go-build` and `~/go/pkg/mod`, one cache per job kind (build,
  lint, and each test lane group), keyed on the Go version and `go.sum` plus
  `tools/go.sum`. Pushes to `main` save one fresh cache per day. PR runs only
  restore, so they cannot churn the repository's 10 GB cache quota.
- golangci-lint: the action's own analysis cache, saved on `main` only.
- npm: `actions/setup-node`'s npm cache, keyed on `web/package-lock.json`.

## Concurrency and merge queue

A new push to a PR cancels that PR's previous CI run. Runs on `main` and in
a merge queue are never cancelled.

`ci.yml` already listens for `merge_group`, so turning on a merge queue for
`main` needs only the branch protection setting. A queue run scopes itself
against the queue's base commit with the same rules as a PR.

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
for its three shards, about 21 of its minutes.

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
