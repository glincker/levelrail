## What changed

<!-- Describe the change. One logical change per PR. -->

## Release note

<!-- One user-facing sentence for the release notes, e.g. "Deploys now
     retry registry pulls that time out." Write NONE if users won't notice
     (CI, tests, refactors). Leave the comment alone to use the PR title.
     Breaking? Add a line: BREAKING CHANGE: what operators must do. -->

## What this doesn't do

<!-- State the scope boundary explicitly, e.g. "does not wire this up
     to the UI yet" or "does not handle the multi-node case". -->

## Test plan

- [ ] `go test ./...` passes
- [ ] `golangci-lint run` passes
- [ ] Added or updated tests covering the change
- [ ] Manually verified the behavior (describe how, if applicable)
