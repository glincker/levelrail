## What broke

<!-- The bug, and its user-visible symptom. Link the issue: Closes #<number> -->

## Root cause

<!-- Why it happened, not just what line changed. -->

## The fix

<!-- What this PR actually does about it. -->

## Regression test

- [ ] Added a test that fails without this fix and passes with it
- [ ] Not applicable, explain why:

## Release note

<!-- One user-facing sentence for the release notes, e.g. "Deploys now
     retry registry pulls that time out." Write NONE if users won't notice
     (CI, tests, refactors). Leave the comment alone to use the PR title.
     Breaking? Add a line: BREAKING CHANGE: what operators must do. -->

## What this doesn't do

<!-- If this is a partial fix or leaves a related issue open, say so. -->

## Test plan

- [ ] `go test ./...` / relevant `web` tests pass
- [ ] `golangci-lint run` / `npx eslint` passes
- [ ] Manually verified the fix (describe how)
