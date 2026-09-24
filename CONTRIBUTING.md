# Contributing to Levelrail

Thanks for taking a look at this project. It's early and moving fast,
so a few conventions keep changes easy to review.

## Before you start

For anything beyond a small fix, open an issue first to talk through the
approach. This avoids spending time on a PR that doesn't fit the current
architecture, especially around the reconciler, the agent transport, or
the database schema, all of which have deliberate design constraints
that aren't always obvious from the code alone.

For quick questions before opening an issue, ask in the `#levelrail`
forum on the [GLINR Discord](https://discord.gg/Ar5pcaZB99).

## Branches and commits

- Branch names: `type/short-description`, e.g. `fix/rollback-image-gc`
  or `feat/agent-reconnect-backoff`.
- Commit messages follow conventional commits: `type: description`, for
  example `fix: prevent image gc from pruning rollback targets`. Common
  types are `feat`, `fix`, `perf`, `refactor`, `docs`, `test`, `chore`.
- No em dashes or en dashes in commit messages or code comments. Use
  commas, periods, or parentheses instead.
- Keep each PR to one logical change. Do not mix a refactor with a
  feature, or bundle unrelated fixes into the same PR. Smaller PRs get
  reviewed faster and are easier to revert if something's wrong.
- State what the PR does not do. If it's a partial implementation
  (e.g. it lays the groundwork but doesn't wire up the UI yet), say so
  explicitly in the description.
- Opening a PR shows the default template. For a type-specific one
  (feature, fix, refactor, docs), append `?template=feature.md` (or
  `fix.md`, `refactor.md`, `docs.md`) to the compare/PR URL.

## Fast dev loop

Iterate on the smallest thing that proves the change, and let CI be the
backstop instead of running it all by hand.

1. **While coding**: test only what you touched, not the whole tree.
   `scripts/affected-tests.sh` runs the changed packages plus their
   dependents; for a single package use `go test -short ./internal/foo`.
   Web: `cd web && npx tsc --noEmit && npx vitest run --changed`.
2. **Prove it works for real**: unit tests can pass on a feature that does
   not work. Boot the actual control plane from your tree and drive it with
   the CLI:

   ```
   scripts/smoke.sh -- nodes list -- attention
   ```

   It builds the binaries, starts a dev-mode server in a throwaway data
   dir (no Docker required), runs each `--` separated CLI command, and
   fails if any exits non-zero. `SMOKE_KEEP=1` leaves the server running
   for manual poking.
3. **Push**: the pre-push hook is a fast lane by default (compile
   everything, then test only the packages you edited; `internal/api` runs
   just the tests from test files you changed; no dependents, no
   `test/e2e`, no coverage gate). It takes seconds. Set
   `LEVELRAIL_PUSH_SCOPE=affected` for the slower run that includes
   dependents and the changed-line coverage gate.
4. **Do not wait on CI**: open the PR, then run
   `gh pr merge --auto --squash`. GitHub merges it when the required
   checks pass. If a check fails, fix it on the same branch; that is the
   only reason to look at CI again.

Dependents, `test/e2e`, and the coverage gate run in CI, and the full
`-race` sweep runs nightly.

## Branch cleanup

`.github/workflows/branch-cleanup.yml` deletes a pull request's branch
when the PR merges, and a weekly sweep removes branches whose PR merged
or was closed unmerged more than 30 days ago. Branches with an open PR
are never touched, and branches with no PR are only listed in the job
summary.

## Secret scanning

[gitleaks](https://github.com/gitleaks/gitleaks) scans for committed
secrets. The pre-commit hook scans only your staged changes (well under a
second) and is skipped with a hint if gitleaks is not installed
(`brew install gitleaks`). CI runs the same scan on every PR and push to
`main` (`.github/workflows/secret-scan.yml`). The dev-mode fixture tokens
in `dev-fixtures.yml` and test fixtures are intentionally public and
allowlisted in `.gitleaks.toml`; extend that file for a genuine false
positive rather than editing real code. Never commit a real credential:
if one leaks, rotate it, since removing it from history does not
un-expose it.

## Running tests

```
go test ./...
```

Table-driven tests are expected for pure logic. Reconcilers need a test
against a fake Docker client covering at least one failure case, since a
reconciler that only handles the happy path isn't done. `/internal` is
held to a 70% coverage floor in CI; new code shouldn't lower it even if
the overall number still passes.

Frontend tests live alongside components in `web/`; see `web/README.md`
for how to run them.

## Flaky tests

CI reruns a failed Go test at most twice (`scripts/ci-go-test.sh`, via
gotestsum) and a failed vitest test at most twice (CI only). A retry is
not a pass: every test that needed one is listed under "Passed only on
retry" or "Flaky Tests" in the job summary, gets a warning annotation,
and on `main` and in the nightly run opens or updates a GitHub issue
labeled `flaky-test`. A test that fails every attempt still fails the
build. Reruns stop entirely when more than 5 tests fail, since that is a
real breakage, and never happen after a data race.

When a test flakes:

1. Fix it if you can. Most flakes are timing: wait on a condition
   (`findBy*`, `waitFor`, a readiness probe, a polled status with a
   deadline) instead of sleeping, and use fake timers instead of real
   ones.
2. If it blocks unrelated work and can't be fixed quickly, quarantine
   it: add a line to `.github/flaky-tests.txt` with its package, test
   name, an open issue URL, and today's date. Quarantined tests are
   skipped in the required lanes and run in a non-blocking lane instead.
3. A quarantined test must be fixed or deleted within 14 days. Overdue
   entries warn on every PR and fail the nightly quarantine job.

The nightly run (`nightly.yml`) also runs everything with `-race` and
`-shuffle=on` to expose order dependence, and repeats the packages with
a flake history (`FLAKE_SWEEP_PACKAGES`) with `-count=3` and no reruns.
To reproduce a shuffled failure, take the `-test.shuffle <seed>` line
from the lane's `events.json` (in its `test-results-*` artifact) and run
`go test -shuffle=<seed> ./internal/pkg/`.

Docker-backed packages (any `*_live_test.go`, `test/e2e`) run in their
own CI job with `-p 2`, so live containers don't compete for the
runner's memory with each other and with the rest of the suite.

## Running the linter

```
golangci-lint run
```

To scope it to one package while iterating:

```
golangci-lint run ./internal/store/...
```

The linter set is deliberately small and fixed (errcheck, govet,
staticcheck, gosec, revive, ineffassign, unparam), configured in
`.golangci.yml`. Don't add linters to that config in a feature PR; raise
it separately if you think the set should change.

## Code style

- Max 500 lines per file (Go) or per component (frontend). Split into
  smaller units rather than growing a file past that.
- No `interface{}` or `any` in exported Go signatures without a comment
  explaining why. Same spirit on the frontend: TypeScript strict mode,
  no `any`.
- Wrap errors with context at every package boundary. Don't return a
  bare `err` across a boundary the caller can't diagnose from.
- Every blocking call takes a `context.Context` and respects
  cancellation.
- Structured logging via `log/slog`. Any log line describing a resource
  includes its ID.
- No `Thread.sleep()`-equivalent busy-waiting in production code paths;
  use proper scheduling or event-driven signaling instead.
- On the frontend: no data fetching inside component bodies, route
  loaders prime the query cache and components read from it. Any list
  that can exceed 50 items is virtualized.

## Architecture decision records

Significant architectural choices are recorded as ADRs under `/adr`, one
per decision, numbered sequentially, including the alternatives that
were considered and why they were rejected. If your change reverses or
meaningfully extends a prior architectural decision, add a new ADR
rather than editing history in place, and reference the one it
supersedes.

## Reporting bugs and requesting features

Use the issue templates. For security issues, see
[SECURITY.md](SECURITY.md) instead of opening a public issue.

## Labels

`type/*` and `size/*` are applied automatically (from the branch name,
changed files, and diff size, see `.github/labeler.yml`). `area/*` is
also automatic, matching whichever part of `/internal`, `/cmd`, or
`/web` a PR touches. The full taxonomy lives in `.github/labels.yml`.
