---
title: "Template upgrades"
description: "Check apps deployed from a catalog template against the current catalog entry. The planner is read-only, and the apply step deploys the new spec."
"og:url": "https://docs.levelrail.com/template-upgrades"
---

Apps deployed from a catalog template can be checked against the current
catalog entry. The planner is read-only; the apply step deploys the new spec,
watches the release and rolls back on failure.

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

## Applying an upgrade

- API: `POST /api/v1/apps/{name}/template-upgrade` with `{"accept_risk": true}` (deploy permission), then `GET /api/v1/apps/{name}/template-upgrade/run` for progress.
- CLI: `levelrail-cli apps upgrade <app> --yes` (without `--yes` it only prints the plan). It follows the run; `--no-wait` returns at once.
- Dashboard: the Upgrade button next to the badge on the app Overview opens the risk diff and shows progress.
- MCP: `get_template_upgrade_plan` reads the plan; it never applies one.

The apply step re-plans first and refuses (HTTP 409 with a code) when:

- the plan has blockers (`upgrade_blocked`),
- the plan is high risk and `accept_risk` is not set (`upgrade_risk_unacknowledged`),
- a named volume of the app has no successful backup newer than the allowed age (`upgrade_backup_required`, lists the volumes),
- the app has services the template does not know (`upgrade_user_services`),
- another upgrade of the app is running (`upgrade_in_progress`).

The new spec is the catalog template with your choices carried over: a pinned
image tag, env values you changed and env variables you added. It is deployed
through the normal compose path, so it appears in compose history as a new
revision. The release is then watched: it succeeds when every service stays
running for the stable period, and fails on a failed deploy attempt, a crash
loop (the stack stays failed or degraded for the configured number of polls) or
a timeout. A failure re-applies the revision that ran before the upgrade as a
rollback revision. Volume data is not rolled back; restore from the backup if a
migration changed it. The run state is kept in memory, so it resets when the
control plane restarts.

| Variable | Default | Meaning |
| --- | --- | --- |
| `APP_TEMPLATE_UPGRADE_BACKUP_REQUIRED` | `true` | Require a fresh backup for every named volume before applying. |
| `APP_TEMPLATE_UPGRADE_BACKUP_MAX_AGE` | `24h` | How old the newest successful volume backup may be. |
| `APP_TEMPLATE_UPGRADE_CHECK_INTERVAL` | `1h` | How often the dashboard re-checks for a newer template. |
| `APP_TEMPLATE_UPGRADE_WATCH_INTERVAL` | `5s` | Poll interval while watching a release. |
| `APP_TEMPLATE_UPGRADE_WATCH_TIMEOUT` | `10m` | Give up and roll back after this long. |
| `APP_TEMPLATE_UPGRADE_STABLE_FOR` | `30s` | How long the stack must stay running to count as healthy. |
| `APP_TEMPLATE_UPGRADE_FAIL_POLLS` | `6` | Consecutive failed or degraded polls that count as a crash loop. |
