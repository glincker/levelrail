---
title: "Which guard catches what"
description: For contributors. The repo-wide guard tests and scripts, what each one fails on, which command runs it, and how to fix the failure.
---

Most of these checks scan the whole tree, so a scoped test run does not reach them. If you touch the area in the first column, run the command in the third before you push. CI mechanics are in [CI](/ci), and the system the guards protect is described in [Architecture](/architecture).

| Guard | Fails when | Run it |
| --- | --- | --- |
| Exec gate | A non-test Go file starts a host process and is not listed | `go test ./test/execgate` |
| Fan-in budget | A package has more transitive dependents than its budget | `scripts/check-fanin.sh` |
| Generated files | A generator output is stale | `scripts/code-gen-check.sh` |
| API reference and OpenAPI | Routes changed and the generated reference did not | `go run ./scripts/gen-api-reference -check` |
| Docs check | A CLI command in docs does not parse, or a relative link is broken | `bash scripts/docs-check.sh --links-only` |
| Dash sweep | An em or en dash is in a tracked text file | `scripts/check-dashes.sh` |
| Migration versions | Two migrations share a version number | `scripts/check-migration-versions.sh` |
| IaC schema | The published resource schema is stale, or a docs example does not validate | `go test ./internal/iac` |
| IaC golden plans | A plan rendering changed | `go test ./internal/iac -run TestGoldenPlans` |
| CLI help flags | A verb defines a flag its `-h` text omits | `go test ./cmd/levelrail-cli -run TestHelpTextCoversEveryFlag` |
| CLI `--json` coverage | A leaf command has no `--json` and no exemption | `go test ./cmd/levelrail-cli -run TestEveryLeafCommandSupportsJSON` |
| CLI completion | The completion command tree drifts from the dispatch switches | `go test ./cmd/levelrail-cli -run Completion` |
| Console allowlist | A risky-looking flag appears on a command the dashboard console allows | `go test ./cmd/levelrail-cli -run Allowlist` |
| MCP token budget | An MCP mode lists more tool tokens than its budget | `scripts/mcp-token-budget.sh` |
| MCP audit tests | An MCP tool is not registered through the audited path | `go test ./internal/mcptools` |
| Windows cross-compile | A unix-only call reaches the CLI build | see below |
| Authz matrix | A route skips authentication, role or resource checks | `go test ./internal/api -run AuthzMatrix` |
| Secret-field guards | A response struct serializes a secret-looking field | `go test ./internal/api -run TestAPIStructsDoNotSerializeSecretLookingFields` |
| Audit coverage | A mutating REST route writes no audit row | `go test ./internal/api -run TestRESTMutatingRoutesAreAudited` |
| Template platform claims | A template claims a platform its images do not list | `go test ./internal/catalog -run Platforms` |
| Locale parity | A translation has extra keys, lost placeholders or dashes | `npx vitest run src/i18n/locales.test.ts` in `web/` |
| New-file i18n check | A new web file has literal JSX text | `scripts/check-i18n.sh <file>` |
| Bundle budgets | A built chunk or the first load grows past its ceiling | `npm run check-bundle-size` in `web/` after a build |
| Icon guards | A web file imports another icon library or a hand-drawn brand mark | `scripts/check-icon-rules.sh` |
| Lint guards | golangci-lint rules, brand strings | see below |
| Git hooks | Format, lint, secrets, dashes, conflict markers, commit format, build and tests | `scripts/install-hooks.sh` |
| Gate scripts | A slow suite fails in the shape CI runs it | `scripts/gate-api.sh`, `scripts/gate-store.sh` |

## Exec gate

`test/execgate` parses every non-test Go file and fails when one calls `os/exec`, `os.StartProcess` or `syscall.Exec` without an entry in `test/execgate/allowlist.txt`. Each entry is `<path> | <reason>`. Docker and BuildKit are driven through their APIs and never belong in the list.

Fix: prefer an API over a subprocess. If a process is unavoidable, add the file with a reason and make sure no user-influenced string (app name, env value, branch, git URL, domain, file name) reaches the argv and that no shell is involved. A reviewer confirms this.

The package is listed in `scripts/ci-guards.txt`, so any non-test Go change selects it in CI.

## IaC schema and docs examples

Two tests in `internal/iac/schema_file_test.go`:

- `TestPublishedSchemaFileIsCurrent` compares `docs/schemas/resources.schema.json` with the schema generated from code. After changing a resource kind, regenerate with `IAC_UPDATE_SCHEMA=1 go test -run TestPublishedSchemaFileIsCurrent ./internal/iac/` and commit the file.
- `TestDocsExamplesValidate` reads every fenced yaml block in `docs/platform-as-code.md` that starts with `version: 1` and runs it through the parser and builder. A renamed field breaks the docs example until both match.

## MCP token budget

`internal/mcptools/budget_test.go` estimates the token cost of `tools/list` for each MCP mode and fails over budget. `scripts/mcp-token-budget.sh` prints the cost per mode. Override a budget with `APP_MCP_TOKEN_BUDGET_<MODE>` while experimenting, not to land a change.

Fix: shorten tool descriptions, drop rarely used parameters from the compact profile, or fold two tools into one. The failure message names the heaviest tools.

## Windows cross-compile

CI builds the CLI for windows (amd64, arm64) and darwin (arm64) with `CGO_ENABLED=0`, because release builds ship those targets and a unix-only call in a shared package would otherwise fail at tag time. Run the same builds locally:

```bash
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -o /dev/null ./cmd/levelrail-cli
GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go build -o /dev/null ./cmd/levelrail-cli
```

Fix: move the unix-only call behind a `_unix.go` file with a build tag and add a Windows stub.

## Authz matrix

`internal/api/authz_matrix_test.go` walks the route table and checks that:

- every route outside the public allowlist returns 401 without credentials,
- a read-only token cannot mutate,
- a resource-scoped deny returns 403,
- the public allowlist itself is current, so a stale entry fails too.

Fix: do not add a route to the public allowlist to make the test pass. Put the route behind the existing authentication and ability checks.

## Secret-field guards

`TestAPIStructsDoNotSerializeSecretLookingFields` in `internal/api/resource_secret_fields_test.go` fails when a JSON-tagged struct gains a field whose name looks like a secret, token or password. Fix: return a `*_set` boolean instead of the value. If the value must be shown once (for example a token on create), add the field to the allowlist in that file with a one-line reason.

## Audit coverage

`TestRESTMutatingRoutesAreAudited` in `internal/api/rest_route_audit_test.go` requires every mutating REST route to write an audit row. The known-gap list is empty on purpose. Do not add to it to silence a new route.

## Icon guards

`scripts/check-icon-rules.sh` scans `web/src` and enforces three rules:

- brand marks come from `@thesvg/react` through `BrandIcon` or `BrandTile`,
- UI icons come from `@thesvg/ui-react`,
- no other icon library, hand-drawn vendor SVG or raster logo.

The pre-commit hook also blocks a staged diff that imports one of the retired icon packages. Run `scripts/check-icon-rules.sh <file>` to check one file.

## Fan-in budget

`scripts/check-fanin.sh` counts, for each internal package, how many packages depend on it directly or not, and fails when the count is over its entry in `scripts/fanin-budget.txt`. An unlisted package falls back to `FANIN_DEFAULT_MAX` (default 20). The count matters because CI selects tests by impact, so a widely imported package makes every change to it expensive.

Fix: split the package or move the new import out of it. Raise a budget entry only with a reason in the PR. `scripts/check-fanin.sh --update` rewrites the file and is for deliberate resets, not for silencing one failure.

## Generated files

`scripts/code-gen-check.sh` runs every checked-in generator (API reference and OpenAPI, template catalog docs, template gallery, template platforms, docs pages, the web route tree) and fails if a tracked file changed. It needs a clean working tree to be meaningful, so commit or stash first. `CODEGEN_SKIP_TEMPLATES=on` skips the template generators for a quick run.

Fix: run the generator the output names, then commit the result. For the API reference run `go run ./scripts/gen-api-reference`, which rewrites `docs/api-reference.md`, `docs/public/openapi.json` and `internal/api/openapi_gen.go`. For a merge conflict in a generated file, regenerate instead of resolving by hand.

## Docs check

`scripts/docs-check.sh` parses every `levelrail-cli` command in `docs/*.md` and `docs/*.mdx` against a freshly built CLI, and checks that relative links resolve. `--links-only` skips the CLI build and runs in seconds. `--commands-only` does the opposite. A failure names the file and the command or link.

Fix: correct the command or flag in the page, or fix the link. Fenced examples of `version: 1` resource files are also validated by the IaC test above, so keep them valid.

## Dash sweep

`scripts/check-dashes.sh` fails on any em or en dash in a tracked text file. The pre-commit hook and the commit-msg hook apply the same rule to staged files and to the commit message. Use commas, periods, parentheses or colons.

## Migration versions

`scripts/check-migration-versions.sh` fails when two files in `internal/store/migrations` share a leading version number. Two branches that both took the next number collide at merge time. `scripts/next-migration.sh` prints the next free number, and `--against-remotes` also warns when another `origin/*` branch uses the same number (`MIGRATION_REMOTE_STRICT=1` makes that a failure). Fix: renumber yours to the next free number.

## CLI drift tests

Four tests in `cmd/levelrail-cli` keep the CLI, its help text, completion and machine output in step.

- `TestHelpTextCoversEveryFlag` fails when a verb defines a flag its `-h` text omits. Existing omissions are frozen in `testdata/help_flag_gaps.txt`. Fix by documenting the flag. The test also fails on a stale line in that file, so delete the line once the flag is documented. Do not add new lines.
- `TestEveryLeafCommandSupportsJSON` fails when a leaf command has no `--json`. A command that prints a script or needs a terminal may be added to `jsonExempt` with a reason. A command that returns data does not belong there.
- `TestCommandTree_MatchesDispatchSwitches` and the other completion tests check that the generated bash, zsh and fish scripts are valid, that they cover the known verbs, and that the command tree matches the dispatch switches. Fix by updating the command tree when you add a verb.
- `console_allowlist_drift_test.go` checks the commands the dashboard console may run. A flag that looks like a path, URL or launch option on an allowlisted command forces a review in `reviewedSafeFlags`.

## IaC golden plans

`internal/iac/golden_test.go` compares plan output with each case's `plan.golden.json`. After an intended change, accept it with `UPDATE_GOLDEN=1 go test ./internal/iac -run TestGoldenPlans` and read the diff before you commit.

## MCP audit tests

`internal/mcptools/audit_coverage_test.go` checks that every API tool registers through the audited path, that mutating tools carry an audit class, and that every registered tool call is audited. `route_audit_test.go` checks that each MCP tool's route is audited on the API side. `budget_headroom_test.go` checks that the compact profile keeps headroom under its budget. Fix by registering the tool through the existing helper, not by adding an exemption.

## Template platform claims

`TestPlatforms_NoUnbackedClaims` in `internal/catalog/platforms_test.go` fails when a template claims a platform that every one of its images does not list in `internal/catalog/platforms.json`. `go run ./scripts/gen-template-platforms` rewrites the arm64 section of the docs from that file, and `scripts/code-gen-check.sh` catches a stale copy. Fix by correcting the claim, not by editing the scan data by hand.

## Locale parity

`web/src/i18n/locales.test.ts` checks the Spanish files against English: no extra keys, the same `{{placeholders}}`, no em or en dashes, and a fallback to English for missing keys. Run it with `npx vitest run src/i18n/locales.test.ts` inside `web/`. Fix by matching the English keys and placeholders. `scripts/check-i18n.sh` is a separate check: it flags literal JSX text in files that are new in the index. End a line with `// i18n-ignore` only for text that is not translatable.

### Rich text and the TypeScript depth limit

`<Trans>` used with a `t` bound to a namespace can hit TypeScript's instantiation depth limit ("Type instantiation is excessively deep") as the number of keys in that namespace grows. The error shows up in `tsc -b`, often in a file you did not touch, once a new key tips the namespace over. Use the split-key pattern instead: write the sentence as separate plain keys and render the link or emphasis in JSX between plain `t()` calls. `web/src/components/DomainEditor.tsx` does this with `editor.descriptionBefore`, `editor.descriptionLink` and `editor.descriptionAfter`. Keep the split points at natural phrase breaks, and keep one sentence's keys together in the locale file so a translator sees them side by side.

## Bundle budgets

`web/scripts/check-bundle-size.js` reads the Vite manifest after a build and fails when the main chunk is over `BUNDLE_SIZE_BUDGET_BYTES` (default 720000), when the first load is over `FIRST_LOAD_BUDGET_BYTES`, or when a chunk is over its entry in `web/bundle-budgets.json`. `npm run check-bundle-size` runs it and then prints the report. After `npm run build`, run it in `web/`. Fix by route-splitting or lazy loading the new code. Raise a budget only with a reason, using `node scripts/bundle-report.js --update` to rewrite the file from the current build.

## Lint guards

- `.golangci.yml` enables `forbidigo` (no `http.DefaultClient` or `http.Get`, use the shared client with the SSRF guard; no `math/rand`, use `crypto/rand` unless it is jitter with a `nolint` reason) and `depguard` (the CLI and MCP server may not import `internal/store` or `internal/docker`; they talk to the API over HTTP). `nolintlint` requires a specific, explained `nolint`.
- `scripts/check-brand-strings.sh` fails on the product name as a string in `kit/` and the Terraform provider. Elsewhere, go through `internal/brand`.

## Git hooks

`scripts/install-hooks.sh` installs three hooks in each clone or worktree.

- `pre-commit` runs on staged files only: gofmt, migration version check (when a migration is staged), gitleaks (skipped with a notice if not installed), go vet and golangci-lint on the changed packages (new issues only), kit and Terraform provider builds, the dash sweep, conflict markers, a limit on long new comment blocks, eslint and prettier for staged web files, and the icon library sweep. It does not run the test suite.
- `commit-msg` requires `<type>(<scope>): <description>` with a type from feat, fix, perf, refactor, docs, test, chore, ci, security, and rejects dashes and AI attribution trailers.
- `pre-push` checks migration versions, regenerates the API reference when routes changed, builds, and runs `go test -short` on the packages your branch changed (not their dependents) plus `tsc -b` for web changes. The base is the nearest `origin/release/*` or `origin/main`.

Environment flags for `pre-push`, all read from the script:

| Variable | Effect |
| --- | --- |
| `LEVELRAIL_PUSH_BACKGROUND=on` | Detach the checks and return at once. Log is `pre-push-last.log` in the git dir. Never used for pushes to main or master. |
| `LEVELRAIL_PUSH_SCOPE=affected` | Also test dependents of the changed packages, instead of the default `fast`. |
| `LEVELRAIL_PUSH_COVERAGE=on` | Run the coverage gates (70 percent aggregate for `internal/`, 50 percent for changed lines). Off by default, CI runs them. |
| `LEVELRAIL_PUSH_WEB_TESTS=on` | Run vitest for related changed web files. |
| `LEVELRAIL_PUSH_BASE` | Override the base ref used to find changed files. |
| `LEVELRAIL_PUSH_TEST_TIMEOUT` | Go test timeout, default 30m. |
| `LEVELRAIL_PREPUSH_API_SHARDS` | Shard count for `internal/api`, default min(4, cores). |
| `LEVELRAIL_PUSH_PR_SIGNAL=off` | Skip the step that comments the result on your open PR. When on, `gh` is authenticated and the PR is not a draft, the hook also arms auto-merge for it, so set this to `off` if you want to review first. |

Related variables: `LEVELRAIL_LINT_CACHE` and `APP_LINT_LOCK_WAIT_SECONDS` (pre-commit lint cache path and lock wait), `LEVELRAIL_HOOK_NODE_HEAP_MB` (heap for `tsc`).

## Gate scripts

These run a slow suite in the shape CI uses, so a local result predicts CI.

- `scripts/gate-api.sh [-r regex]` runs `internal/api` with `-short` in the same lanes as CI (`GATE_API_LANES`, default 4), with bounded concurrency (`GATE_API_CONCURRENCY`) and a timeout (`GATE_API_TIMEOUT`, default 25m). It prints failures and one summary line. It leaves out `internal/api` tests listed in `.github/flaky-tests.txt`.
- `scripts/gate-store.sh [-r regex] [-R]` runs `internal/store` with `-short`, optionally with `-race`, and prints failures and the slowest tests. `GATE_STORE_TIMEOUT` defaults to 25m.

Both take minutes on a full run. Use `-r` with a test name to stay under a minute while iterating.
