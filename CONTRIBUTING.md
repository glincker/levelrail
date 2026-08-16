# Contributing to Levelrail

Thanks for taking a look at this project. It's early and moving fast,
so a few conventions keep changes easy to review.

## Before you start

For anything beyond a small fix, open an issue first to talk through the
approach. This avoids spending time on a PR that doesn't fit the current
architecture, especially around the reconciler, the agent transport, or
the database schema, all of which have deliberate design constraints
that aren't always obvious from the code alone.

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
