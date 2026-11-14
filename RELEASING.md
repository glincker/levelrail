# Releasing Levelrail

Status: the checks below are report-only until the owner enables enforcement.

## 1. Principles

- A version is computed from commit types and diffs. Nobody picks a number.
- `Release-As:` is a repair tool, not a way to choose a nicer number.
- `main` is locked. One `release/{version}` branch at a time, one squash PR.

## 2. Bump table

Stable surface: HTTP API, CLI flags and exit codes, stable IaC fields, config keys, Terraform provider and SDK contracts, migrations that need operator action.

| Change | Commit | Bump |
| --- | --- | --- |
| Fix, perf, security fix | `fix:` `perf:` `security:` | patch |
| New capability, flag, API field, template, beta resource | `feat:` | minor |
| Deprecation of a stable item (still works) | `feat:` plus a `Deprecated:` line | minor |
| Removal or incompatible change of a stable item | `type!:` plus `BREAKING CHANGE:` footer | major |
| Any change to an item marked `x-stability: beta` | `feat:` or `fix:`, never `!` | minor or patch |
| Promote beta to stable | `feat:` | minor |

`docs`, `chore`, `ci`, `test` and `refactor` alone produce no release. The release takes the highest bump among its commits.

Beta markers live in the IaC schema (`internal/iac/schema.go`, published in `internal/iac/schema/resources.schema.json`) as `x-stability`.

## 3. Daily flow

- Work on the open `release/{version}` branch. Use a real type per commit, `type: outcome`.
- Mark a break with `!` and a `BREAKING CHANGE:` footer. Never use `!` on beta-only changes.

## 4. Cutting a release

1. Open the one squash PR from the release branch into `main`. Build the body with `scripts/release-squash-message.sh <version>` so every user-visible change keeps its type.
2. Wait for CI once, merge.
3. release-please opens its release PR. `release-pr-generated` regenerates `docs/changelog.mdx` on it.
4. Merge the release PR. The tag triggers `release.yml`.

## 5. Hotfix

Branch `hotfix/<name>` from `main`, one `fix:` commit, PR to `main`. Only on owner request. No `feat:` and no `!`. Merge `main` into the open release branch afterwards, never the reverse.

## 6. Release-As policy

- Only the owner, only to fix a wrong computed version or for a milestone decision.
- The version must exceed the current one and skip at most one minor or major.
- Remove any `release-as` key from the config after the release.

## 7. CI checks

- `commit-semver` (report-only): runs `scripts/check-commit-semver.mjs` per commit. Findings show as warnings. A finding means the type and the diff disagree: fix the type or the change. Set `SEMVER_ENFORCE=1` to make it fail.
- `release-pr-generated`: pushes the regenerated changelog page to the `release-please--` branch. Needs the release App (`RELEASE_APP_CLIENT_ID` variable, `RELEASE_APP_PRIVATE_KEY` secret) or the `RELEASE_PLEASE_TOKEN` secret. Without them it skips with a notice.

## 8. Troubleshooting

- Wrong version: look for a commit with a wrong type or a stray `Release-As:`, and for `last-release-sha` in the release-please config.
- CI does not run on the release PR or a pushed commit: the push used the default token. Configure the App or PAT above.
- `publish-npm` red: npm trusted publishing is not configured for that package, see `NPM_PUBLISH_ENABLED`.

## 9. kit module

`kit` is versioned on its own by the same rules and its own diff. A breaking kit change needs an ADR first because Go requires a new major module path.
