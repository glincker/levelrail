# Review guide

For contributors and review agents. It condenses the rules in [AGENTS.md](AGENTS.md), [CONTRIBUTING.md](.github/CONTRIBUTING.md) and [Which guard catches what](docs/contributing-guards.md). When they disagree with this page, they win.

## Before you open a PR

- Run `scripts/preflight.sh`. It picks the guards your diff needs and the pre-push hook runs it too.
- One logical change per PR, and say what the PR does not do.
- Commit subjects are `<type>: <description>` with type `feat`, `fix`, `perf`, `refactor`, `docs`, `test`, `chore`, `ci` or `security`.

## What to check, most serious first

1. **Authz.** Every new or changed route checks authentication, role and the resource it touches. Run `go test ./internal/api -run AuthzMatrix`. A mutating REST route also writes an audit row (`TestRESTMutatingRoutesAreAudited`).
2. **Secrets.** No token, password or key in code, tests, logs, docs or fixtures. Response structs must not serialize secret-looking fields (`TestAPIStructsDoNotSerializeSecretLookingFields`). Tokens come from the environment, never from a file. gitleaks runs in the pre-commit hook and in CI.
3. **Host processes.** Non-test Go that starts a process needs a reviewed entry in `test/execgate/allowlist.txt`, no shell, and no user-influenced string in argv. Docker and BuildKit go through their APIs.
4. **Migrations.** Versions are unique and increasing, taken from `scripts/next-migration.sh`, and re-checked on the integration branch before pushing (`scripts/check-migration-versions.sh`). A shipped migration is never edited. Table rebuilds and drops need their `-- migrate:` marker lines.
5. **Reconcilers.** Code under `internal/reconcile` is level-triggered and idempotent, with a test for the half-succeeded case.
6. **Errors and context.** Errors are wrapped with context at package boundaries. Every blocking call takes a `context.Context`.
7. **Generated files.** Never hand-edit the API reference, `docs/public/openapi.json`, the template catalog or the MCP tool surface doc. Regenerate and commit the output. `scripts/code-gen-check.sh` must be clean.
8. **Dashboard strings.** Text goes through `useTranslation()` and `web/src/locales/en/*.json`, with no literal JSX text (`scripts/check-i18n.sh <file>`). Only the `common` namespace loads at boot, so a new string in a first-load file belongs in a lazy namespace unless it must be there. Locale parity must hold.
9. **Bundle budgets.** After a web build, `npm run check-bundle-size` in `web/` passes. Fix growth by route-splitting or lazy loading. Raise a budget only with a stated reason.
10. **Web rules.** No `any`, no inline styles, no data fetching in component bodies, icons only from `@thesvg/ui-react`, lists over 50 items virtualized.
11. **Names and thresholds.** Product and binary names come from `internal/brand`. Limits and thresholds come from configuration, not literals.
12. **Reach.** A feature is not done until it works from the dashboard, the CLI and the API, and the docs and CLI help changed in the same PR.

## Tests

- Keep tests lean and independent. Each test sets up its own state and does not rely on order or on another test's data. Do not add a test only to raise a number.
- A bug fix includes a test that fails without the fix.
- Wait on a condition, not a sleep. Use fake timers in web tests.
- Test the slice you touched (`scripts/test-slice.sh`, `go test -short ./internal/foo`), then prove the feature with `scripts/smoke.sh`.

## Text and source hygiene

- No em dashes or en dashes anywhere (`scripts/check-dashes.sh`).
- Comments default to none. Add one only for a non-obvious why, in one to three lines.
- No other project's name in source code. Comparisons belong in docs that cite public sources.
- Files stay under 500 lines.

## Reviewer output

Report findings in this order: authz, secrets, data loss, then correctness, then style. Give the file and line, the failing case, and the smallest fix. Say what you did not check.
