# Template upgrades

Apps deployed from a catalog template can be checked against the current
catalog entry. This release ships the read-only planner. Applying an upgrade
is the next slice.

## Reading the plan

- API: `GET /api/v1/apps/{name}/template-upgrade` (read permission on the app).
- CLI: `levelrail-cli apps upgrade-plan <app>`.
- Dashboard: the app Overview shows "Template update available" when the plan has changes.

The plan lists names and image tags only. Environment values, including
generated secrets, are never included.

## What the planner compares

The deployed compose revision is compared with the current catalog template.
The first template revision of the app, when it exists, is the baseline: a
value that differs from the baseline is a user edit and is kept
(`keep_user`). Without a baseline every differing value is treated as a user
edit, so nothing is ever overwritten.

| Area | Rule |
| --- | --- |
| Image | Same repository and tag suffix with semver tags is classified patch, minor or major (a lower tag is a downgrade). Anything else is `changed`. A tag the user changed from the baseline is `preserved`. |
| Env | New variables are `add` (generated secrets and required inputs are flagged, by name). Variables the template dropped are `remove`. Variables the user set are kept. Generated values are never rotated. |
| Volumes | Named volumes only. A removed or retargeted volume is a blocker. |
| Services | A new service is `added`. A removed service that owns a named volume is a blocker. |
| Ports, healthcheck | Additions and removals are listed. |

Domains, resources and bind mounts are user-owned and never part of a plan.

## Risk

- `none`: nothing to do.
- `low`: patch or minor image moves only.
- `medium`: also env additions or updates, volume additions, port or healthcheck changes.
- `high`: a major or unclassifiable image change, a downgrade, removed env vars, a new or removed service, or any blocker.

`automatic` is true only when there are changes, no blockers and the risk is not high.

## For the apply slice

`templateupgrade.Build` is pure, so an executor can call it again right before
applying. The intended order: take a backup of the app's volumes, re-plan,
refuse if `blockers` is not empty or `automatic` is false, apply `services` in
order (changed first, added last), keep entries marked `preserved` or
`keep_user` untouched, and roll back to the previous compose revision on
failure. The plan has a stable order, so it can be recorded in the audit log
as is.
