## What changed

<!-- Structural change, no intended behavior change. -->

## Why

<!-- What made this worth doing now: readability, a file over 500 lines,
     duplicated logic, a pattern that needs to be reused elsewhere. -->

## Behavior

- [ ] Confirmed no behavior change (existing tests pass unmodified)
- [ ] Behavior does change in a small, called-out way, describe it:

## Release note

<!-- One user-facing sentence for the release notes, e.g. "Deploys now
     retry registry pulls that time out." Write NONE if users won't notice
     (CI, tests, refactors). Leave the comment alone to use the PR title.
     Breaking? Add a line: BREAKING CHANGE: what operators must do. -->

## What this doesn't do

<!-- Explicitly not a feature or fix PR; state anything deliberately left alone. -->

## Test plan

- [ ] `go test ./...` / relevant `web` tests pass, unmodified
- [ ] `golangci-lint run` / `npx eslint` passes
