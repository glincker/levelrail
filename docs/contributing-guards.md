---
title: "Which guard catches what"
description: For contributors. The repo-wide guard tests and scripts, what each one fails on, which command runs it, and how to fix the failure.
---

Most of these checks scan the whole tree, so a scoped test run does not reach them. If you touch the area in the first column, run the command in the third before you push. CI mechanics are in [CI](/ci).

| Guard | Fails when | Run it |
| --- | --- | --- |
| Exec gate | A non-test Go file starts a host process and is not listed | `go test ./test/execgate` |
| IaC schema | The published resource schema is stale, or a docs example does not validate | `go test ./internal/iac` |
| MCP token budget | An MCP mode lists more tool tokens than its budget | `scripts/mcp-token-budget.sh` |
| Windows cross-compile | A unix-only call reaches the CLI build | see below |
| Authz matrix | A route skips authentication, role or resource checks | `go test ./internal/api -run AuthzMatrix` |
| Secret-field guards | A response struct serializes a secret-looking field | `go test ./internal/api -run TestAPIStructsDoNotSerializeSecretLookingFields` |
| Audit coverage | A mutating REST route writes no audit row | `go test ./internal/api -run TestRESTMutatingRoutesAreAudited` |
| Icon guards | A web file imports another icon library or a hand-drawn brand mark | `scripts/check-icon-rules.sh` |
| Lint guards | golangci-lint rules, brand strings, dashes, fan-in budget | see below |

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

## Lint guards

- `.golangci.yml` enables `forbidigo` (no `http.DefaultClient` or `http.Get`, use the shared client with the SSRF guard; no `math/rand`, use `crypto/rand` unless it is jitter with a `nolint` reason) and `depguard` (the CLI and MCP server may not import `internal/store` or `internal/docker`; they talk to the API over HTTP). `nolintlint` requires a specific, explained `nolint`.
- `scripts/check-brand-strings.sh` fails on the product name as a string in `kit/` and the Terraform provider. Elsewhere, go through `internal/brand`.
- `scripts/check-dashes.sh` fails on em or en dashes in tracked text files.
- `scripts/check-i18n.sh` fails on literal JSX text in new web files. Use `t()` with a key in `web/src/locales/en`.
- `scripts/check-migration-versions.sh` fails on duplicate or out-of-order migration versions.
- `scripts/check-fanin.sh` fails when a package is imported, directly or not, by more packages than its entry in `scripts/fanin-budget.txt` allows.
- `scripts/docs-check.sh` checks that CLI commands in docs parse against the real CLI and that relative links resolve.

## Install the hooks

`scripts/install-hooks.sh` installs pre-commit, commit-msg and pre-push in each clone or worktree. Most of the scripts above run from pre-commit on the files you staged.
