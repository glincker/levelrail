## Summary

<!-- One paragraph: what this changes and why. One theme per PR. -->

## Change ID and tasks

- Tasks: <!-- task numbers from the parity task list, e.g. 118, 123 -->
- ADR: <!-- ADR number if any, else none -->

## Added / Changed / Removed / Fixed

<!-- Bullets that name the verb and the thing: route, setting key, flag, table or UI page. -->

- Added:
- Changed:
- Removed:
- Fixed:

## Surfaces touched

<!-- A capability must reach API, CLI and Dashboard, or say here why not. -->

- [ ] API
- [ ] CLI
- [ ] Dashboard
- [ ] Agent
- [ ] Docs
- Why a surface is missing:

## Migrations

- [ ] None
- [ ] Numbers: <!-- e.g. 0123 -->
- [ ] Forward only, no down migration
- [ ] Upgrade test from the previous release passes

## Runtime settings added

<!-- New tunables are runtime settings (API, CLI, dashboard), env only seeds defaults. -->

- [ ] None
- Key / default / applies live or needs restart:

## Feature flags and status

- [ ] No flag
- Flag:
- Label: <!-- experimental / beta / stable -->
- [ ] Row added or updated in docs/feature-status.md with evidence

## Security review

<!-- Write "none" explicitly if nothing applies. -->

- [ ] None
- [ ] Authz deny tests added for new routes
- [ ] No secrets in code, tests, logs or docs
- [ ] Host exec reviewed (execgate)
- [ ] Input bounds and size limits checked

## Behaviour changes and upgrade notes

<!-- What an operator sees differently after upgrading. Write "none" if nothing. -->

## Not verified / needs a real host

<!-- Anything unproven here: real servers, DNS, TLS, multi-node, cloud accounts. -->

## Tests run

<!-- Targeted only, e.g. go test -run TestFoo ./internal/bar -->

## Rollback

<!-- How to undo: revert, flag off, setting back, data caveats. -->
