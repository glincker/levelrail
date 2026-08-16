## What changed

<!-- Describe the feature. One logical change per PR. -->

## Why

<!-- What problem does this solve, for whom? -->

## Surface parity

<!-- Per CLAUDE.md: a feature that's API-only, with no UI or CLI path
     to it, isn't done. Check what applies. -->

- [ ] Wired into the UI
- [ ] Wired into the CLI
- [ ] Neither applies (internal-only mechanism, no operator-facing action), explain why:

## What this doesn't do

<!-- State the scope boundary explicitly. -->

## Test plan

- [ ] `go test ./...` / relevant `web` tests pass
- [ ] `golangci-lint run` / `npx eslint` passes
- [ ] Added or updated tests covering the change
- [ ] Manually verified the behavior (describe how)
